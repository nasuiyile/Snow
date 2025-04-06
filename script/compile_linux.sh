#!/bin/bash
# 编译 Windows、macOS 和 Linux 平台的 64 位二进制文件
platforms=("linux/amd64" "darwin/amd64" "windows/amd64")
for platform in "${platforms[@]}"; do
  GOOS=${platform%/*} GOARCH=${platform#*/} go build -o "myprogram-${platform}" main.go
done