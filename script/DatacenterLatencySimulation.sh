
#设置99.5%的请求为 0.8ms为中心的正态分布标准差为0.015,
 #0.5%的请求为范围为0.25-5,scale为1.5的指数分布，

#清楚所有规则
sudo tc qdisc del dev lo root 2>/dev/null || true


#创建具有3个子类的PRIO队列（1:1, 1:2, 1:3）
sudo tc qdisc add dev lo root handle 1: prio bands 3

#正态分布队列（99.5%的流量）
sudo tc qdisc add dev lo parent 1:1 handle 10: netem \
    delay 800us 15us distribution normal

#    指数分布队列（0.5%的流量）
sudo tc qdisc add dev lo parent 1:2 handle 20: netem \
    delay 250us 4750us distribution exponential



#设置iptables标记规则
# 创建自定义链
sudo iptables -t mangle -N PORT_MARK
sudo iptables -t mangle -A PORT_MARK \
    -m statistic --mode random --probability 0.995 \
    -j MARK --set-mark 1
sudo iptables -t mangle -A PORT_MARK \
    -m mark --mark 0 \
    -j MARK --set-mark 2

# 应用标记规则
sudo iptables -t mangle -A OUTPUT -p tcp --dport 8100 -j PORT_MARK
sudo iptables -t mangle -A PREROUTING -p tcp --dport 8100 -m mark --mark 0 -j PORT_MARK


# 添加TC过滤器
sudo tc filter add dev lo parent 1: protocol ip prio 1 \
    u32 match mark 1 0xffff flowid 1:1
sudo tc filter add dev lo parent 1: protocol ip prio 1 \
    u32 match mark 2 0xffff flowid 1:2
sudo tc filter add dev lo parent 1: protocol ip prio 1 \
    u32 flowid 1:3  # 默认流量（无延迟）