#断开时间减少
sudo sysctl -w net.ipv4.tcp_fin_timeout=1
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
#设置更多的连接数
ulimit -n 1048576
mkdir dataset

chmod +x ./web-linux
chmod +x ./churn-linux