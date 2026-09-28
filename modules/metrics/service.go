package metrics

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

// DefaultTTL 渲染缓存 TTL：也是「最短采集间隔」。
// 15s 与 Prometheus 常见抓取间隔一致，取值再小只会重复打业务库。
const DefaultTTL = 15 * time.Second

// Service 把采集结果渲染成 Prometheus 文本格式并带 TTL 缓存，
// 避免每次抓取都去查业务库。
type Service struct {
	collector *Collector
	ttl       time.Duration
	logger    *log.Logger

	mu       sync.Mutex
	body     []byte
	rendered time.Time
}

// NewService 新建服务。ttl <= 0 时取 DefaultTTL。
func NewService(c *Collector, ttl time.Duration, logger *log.Logger) *Service {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if logger == nil {
		logger = log.New(log.Writer(), "[filscan-metrics] ", log.LstdFlags)
	}
	return &Service{collector: c, ttl: ttl, logger: logger}
}

// Render 返回指标文本。force=true 时忽略缓存立即采集。
func (s *Service) Render(ctx context.Context, force bool) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !force && s.body != nil && time.Since(s.rendered) < s.ttl {
		return s.body
	}
	set := s.collector.Collect(ctx)
	body := set.Bytes()
	if body == nil {
		// 渲染失败只可能来自 Add 的 HELP/TYPE 冲突（编码期问题），如实返回空并记日志。
		s.logger.Printf("渲染指标失败：指标族 HELP/TYPE 冲突")
		return []byte{}
	}
	s.body = body
	s.rendered = time.Now()
	return body
}

// Handler 返回 /metrics 的 http.Handler（Prometheus 文本格式）。
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), s.ttl)
		defer cancel()
		body := s.Render(ctx, false)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			s.logger.Printf("写响应失败: %s", err)
		}
	})
}

// Listen 在 addr 上提供 /metrics 与 /healthz，直到 ctx 结束。
func (s *Service) Listen(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", s.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	s.logger.Printf("监听 %s，指标路径 /metrics", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
