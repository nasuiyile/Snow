go run ./tool/benchmark/brokendown -configPath=/mnt/e/code/go/Snow/config/config.yml
go run ./tool/benchmark


go build -o web.exe ./tool/benchmark

#linux
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o web-linux ./tool/benchmark

$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o stable-linux ./tool/benchmark/stable

$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o churn-linux ./tool/benchmark/churn

$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o brokendown-linux ./tool/benchmark/brokendown


#export
curl 127.0.0.1/export