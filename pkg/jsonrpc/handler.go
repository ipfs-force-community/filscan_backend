package jsonrpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	
	"github.com/gozelle/opencensus/stats"
	"github.com/gozelle/opencensus/tag"
	"github.com/gozelle/opencensus/trace"
	"github.com/gozelle/opencensus/trace/propagation"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/xerrors"
	
	"github.com/gozelle/jsonrpc/metrics"
)

type rpcHandler struct {
	paramReceivers []reflect.Type
	nParams        int
	
	receiver    reflect.Value
	handlerFunc reflect.Value
	
	hasCtx int
	
	errOut int
	valOut int
}

// Request / response

type request struct {
	//Jsonrpc string            `json:"jsonrpc"`
	ID     interface{}       `json:"id,omitempty"`
	Method string            `json:"method"`
	Params []param           `json:"params"`
	Meta   map[string]string `json:"meta,omitempty"`
}

// Limit request size. Ideally this limit should be specific for each field
// in the JSON request but as a simple defensive measure we just limit the
// entire HTTP body.
// Configured by WithMaxRequestSize.
const DEFAULT_MAX_REQUEST_SIZE = 100 << 20 // 100 MiB

type response struct {
	*Error
	ID interface{} `json:"id"`
	//Jsonrpc string      `json:"jsonrpc,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

// Register

func (s *RPCServer) register(namespace string, r interface{}) {
	val := reflect.ValueOf(r)
	// TODO: expect ptr
	
	for i := 0; i < val.NumMethod(); i++ {
		method := val.Type().Method(i)
		
		funcType := method.Func.Type()
		hasCtx := 0
		if funcType.NumIn() >= 2 && funcType.In(1) == contextType {
			hasCtx = 1
		}
		
		ins := funcType.NumIn() - 1 - hasCtx
		recvs := make([]reflect.Type, ins)
		for i := 0; i < ins; i++ {
			recvs[i] = method.Type.In(i + 1 + hasCtx)
		}
		
		valOut, errOut, _ := processFuncOut(funcType)
		
		s.methods[namespace+"."+method.Name] = rpcHandler{
			paramReceivers: recvs,
			nParams:        ins,
			
			handlerFunc: method.Func,
			receiver:    val,
			
			hasCtx: hasCtx,
			
			errOut: errOut,
			valOut: valOut,
		}
	}
}

// Handle

type rpcErrFunc func(w func(func(io.Writer)), req *request, code ErrorCode, err error)
type chanOut func(reflect.Value, interface{}) error

func (s *RPCServer) handleReader(ctx context.Context, r io.Reader, w io.Writer, rpcError rpcErrFunc) {
	wf := func(cb func(io.Writer)) {
		cb(w)
	}
	
	var req request
	// We read the entire request upfront in a buffer to be able to tell if the
	// client sent more than maxRequestSize and report it back as an explicit error,
	// instead of just silently truncating it and reporting a more vague parsing
	// error.
	bufferedRequest := new(bytes.Buffer)
	// We use LimitReader to enforce maxRequestSize. Since it won't return an
	// EOF we can't actually know if the client sent more than the maximum or
	// not, so we read one byte more over the limit to explicitly query that.
	// FIXME: Maybe there's a cleaner way to do this.
	reqSize, err := bufferedRequest.ReadFrom(io.LimitReader(r, s.maxRequestSize+1))
	if err != nil {
		// ReadFrom will discard EOF so any error here is unexpected and should
		// be reported.
		rpcError(wf, &req, rpcParseError, xerrors.Errorf("reading request: %w", err))
		return
	}
	if reqSize > s.maxRequestSize {
		rpcError(wf, &req, rpcParseError,
			// rpcParseError is the closest we have from the standard errors defined
			// in [jsonrpc spec](https://www.jsonrpc.org/specification#error_object)
			// to report the maximum limit.
			xerrors.Errorf("request bigger than maximum %d allowed",
				s.maxRequestSize))
		return
	}
	
	if err := json.NewDecoder(bufferedRequest).Decode(&req); err != nil {
		rpcError(wf, &req, rpcParseError, xerrors.Errorf("unmarshaling request: %w", err))
		return
	}
	
	if req.ID, err = normalizeID(req.ID); err != nil {
		rpcError(wf, &req, rpcParseError, xerrors.Errorf("failed to parse ID: %w", err))
		return
	}
	
	s.handle(ctx, req, wf, rpcError, func(bool) {}, nil)
}

func doCall(methodName string, f reflect.Value, params []reflect.Value) (out []reflect.Value, err error) {
	defer func() {
		if i := recover(); i != nil {
			err = xerrors.Errorf("panic in rpc method '%s': %s", methodName, i)
			log.Desugar().WithOptions(zap.AddStacktrace(zapcore.ErrorLevel)).Sugar().Error(err)
		}
	}()
	
	out = f.Call(params)
	return out, nil
}

func (s *RPCServer) getSpan(ctx context.Context, req request) (context.Context, *trace.Span) {
	if req.Meta == nil {
		return ctx, nil
	}
	if eSC, ok := req.Meta["SpanContext"]; ok {
		bSC := make([]byte, base64.StdEncoding.DecodedLen(len(eSC)))
		_, err := base64.StdEncoding.Decode(bSC, []byte(eSC))
		if err != nil {
			log.Errorf("SpanContext: decode", "error", err)
			return ctx, nil
		}
		sc, ok := propagation.FromBinary(bSC)
		if !ok {
			log.Errorf("SpanContext: could not create span", "data", bSC)
			return ctx, nil
		}
		ctx, span := trace.StartSpanWithRemoteParent(ctx, "api.handle", sc)
		span.AddAttributes(trace.StringAttribute("method", req.Method))
		return ctx, span
	}
	return ctx, nil
}

func (s *RPCServer) createError(err error) *Error {
	
	var out *Error
	
	if e, ok := err.(*Error); ok {
		out = &Error{
			Code:    ErrorCode(e.Code),
			Message: e.Message,
			Detail:  e.Detail,
		}
		return out
	}
	
	var code ErrorCode = 1
	if s.errors != nil {
		c, okk := s.errors.byType[reflect.TypeOf(err)]
		if okk {
			code = c
		}
	}
	
	out = &Error{
		Code:    code,
		Message: err.(error).Error(),
	}
	
	if m, okk := err.(marshalable); okk {
		meta, ee := m.MarshalJSON()
		if ee == nil {
			out.Meta = meta
		}
	}
	
	return out
}

func (s *RPCServer) handle(ctx context.Context, req request, w func(func(io.Writer)), rpcError rpcErrFunc, done func(keepCtx bool), chOut chanOut) {
	// Not sure if we need to sanitize the incoming req.Method or not.
	ctx, span := s.getSpan(ctx, req)
	ctx, _ = tag.New(ctx, tag.Insert(metrics.RPCMethod, req.Method))
	defer span.End()
	
	handler, ok := s.methods[req.Method]
	if !ok {
		aliasTo, ok := s.aliasedMethods[req.Method]
		if ok {
			handler, ok = s.methods[aliasTo]
		}
		if !ok {
			rpcError(w, &req, rpcMethodNotFound, fmt.Errorf("method '%s' not found", req.Method))
			stats.Record(ctx, metrics.RPCInvalidMethod.M(1))
			done(false)
			return
		}
	}
	
	if len(req.Params) != handler.nParams {
		rpcError(w, &req, rpcInvalidParams, fmt.Errorf("wrong param count (method '%s'): %d != %d", req.Method, len(req.Params), handler.nParams))
		stats.Record(ctx, metrics.RPCRequestError.M(1))
		done(false)
		return
	}
	
	outCh := handler.valOut != -1 && handler.handlerFunc.Type().Out(handler.valOut).Kind() == reflect.Chan
	defer done(outCh)
	
	if chOut == nil && outCh {
		rpcError(w, &req, rpcMethodNotFound, fmt.Errorf("method '%s' not supported in this mode (no out channel support)", req.Method))
		stats.Record(ctx, metrics.RPCRequestError.M(1))
		return
	}
	
	callParams := make([]reflect.Value, 1+handler.hasCtx+handler.nParams)
	callParams[0] = handler.receiver
	if handler.hasCtx == 1 {
		callParams[1] = reflect.ValueOf(ctx)
	}
	
	for i := 0; i < handler.nParams; i++ {
		var rp reflect.Value
		
		typ := handler.paramReceivers[i]
		dec, found := s.paramDecoders[typ]
		if !found {
			rp = reflect.New(typ)
			if err := json.NewDecoder(bytes.NewReader(req.Params[i].data)).Decode(rp.Interface()); err != nil {
				rpcError(w, &req, rpcParseError, xerrors.Errorf("unmarshaling params for '%s' (param: %T): %w", req.Method, rp.Interface(), err))
				stats.Record(ctx, metrics.RPCRequestError.M(1))
				return
			}
			rp = rp.Elem()
		} else {
			var err error
			rp, err = dec(ctx, req.Params[i].data)
			if err != nil {
				rpcError(w, &req, rpcParseError, xerrors.Errorf("decoding params for '%s' (param: %d; custom decoder): %w", req.Method, i, err))
				stats.Record(ctx, metrics.RPCRequestError.M(1))
				return
			}
		}
		
		callParams[i+1+handler.hasCtx] = reflect.ValueOf(rp.Interface())
	}
	
	// /////////////////
	
	callResult, err := doCall(req.Method, handler.handlerFunc, callParams)
	if err != nil {
		rpcError(w, &req, 0, xerrors.Errorf("fatal error calling '%s': %w", req.Method, err))
		stats.Record(ctx, metrics.RPCRequestError.M(1))
		return
	}
	if req.ID == nil {
		return // notification
	}
	
	// /////////////////
	
	resp := response{
		//Jsonrpc: "2.0",
		ID: req.ID,
	}
	
	var respErr *Error
	if handler.errOut != -1 {
		err := callResult[handler.errOut].Interface()
		
		if err != nil {
			log.Warnf("error in RPC call to '%s': %+v", req.Method, err)
			stats.Record(ctx, metrics.RPCResponseError.M(1))
			
			respErr = s.createError(err.(error))
		}
	}
	
	var kind reflect.Kind
	var res interface{}
	var nonZero bool
	if handler.valOut != -1 {
		res = callResult[handler.valOut].Interface()
		kind = callResult[handler.valOut].Kind()
		nonZero = !callResult[handler.valOut].IsZero()
	}
	
	// check error as JSON-RPC spec prohibits error and value at the same time
	if respErr == nil {
		if res != nil && kind == reflect.Chan {
			// Channel responses are sent from channel control goroutine.
			// Sending responses here could cause deadlocks on writeLk, or allow
			// sending channel messages before this rpc call returns
			
			//noinspection GoNilness // already checked above
			err = chOut(callResult[handler.valOut], req.ID)
			if err == nil {
				return // channel goroutine handles responding
			}
			
			log.Warnf("failed to setup channel in RPC call to '%s': %+v", req.Method, err)
			stats.Record(ctx, metrics.RPCResponseError.M(1))
			respErr = &Error{
				Code:    1,
				Message: err.(error).Error(),
			}
		} else {
			resp.Result = res
		}
	} else {
		resp.Error = respErr
		if nonZero {
			log.Errorw("error and res returned", "request", req, "r.err", respErr, "res", res)
		}
	}
	
	w(func(w io.Writer) {
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error(err)
			stats.Record(ctx, metrics.RPCResponseError.M(1))
			return
		}
	})
}
