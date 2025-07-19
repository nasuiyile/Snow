# 1. 清除现有规则
sudo tc qdisc del dev lo root

# 2. 添加主队列规则
sudo tc qdisc replace  dev lo root handle 1: prio

# 3. 为特定端口添加延迟
sudo tc qdisc replace  dev lo parent 1:1 handle 10: netem delay 500us

# 4. 设置过滤器，匹配多个目标端口
sudo tc filter replace  dev lo protocol ip parent 1:0 prio 1 u32 match ip dport 8100 0xffff flowid 1:1
sudo tc filter replace  dev lo protocol ip parent 1:0 prio 1 u32 match ip dport 8081 0xffff flowid 1:1
sudo tc filter replace  dev lo protocol ip parent 1:0 prio 1 u32 match ip dport 8082 0xffff flowid 1:1




curl -o /dev/null -s -w "DNS解析时间: %{time_namelookup}\n建立连接时间: %{time_connect}\nSSL握手时间: %{time_appconnect}\n准备传输时间: %{time_pretransfer}\n开始传输时间: %{time_starttransfer}\n总时间: %{time_total}\n" http://127.0.0.1::80

curl -o /dev/null -s -w "%{time_total}\n" 127.0.0.1:8100  | awk '{print $1 * 1000}'


./latency-tool  -port=8100 -latency=300