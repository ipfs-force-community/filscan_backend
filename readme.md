# Filscan

## 更新依赖注入

```bash
make all
```

## 项目运行

```bash
make all && go run cmd/filscan/filscan.go cmd/filscan/wire_gen.go -c configs/local.toml

# 或

make build
./bin/filscan -c configs/local.toml
```

## API Mock 模式

配置文件:
```
mock_mode = true
```