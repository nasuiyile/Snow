#断开时间减少
sudo sysctl -w net.ipv4.tcp_fin_timeout=1
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
#设置更多的连接数
ulimit -n 1048576
mkdir dataset

chmod +x ./web-linux
chmod +x ./brokendown-linux
chmod +x ./churn-linux
chmod +x ./stable-linux

#设置tcp
sudo vim /etc/security/limits.conf
#* soft nofile 1048576000 #设置文件句柄数
#* hard nofile 1048576000

#运行守护进程
nohup ./web-linux > web.log 2>&1 &
nohup ./stable-linux > stable.log 2>&1 &
tail -f stable.log
#杀死后台进程
kill -9 $(lsof -t -i:20000)
