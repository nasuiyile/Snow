#!/bin/sh

cd /tmp || { echo "无法切换到/root目录"; exit 1; }
rm ping_monitor.log
MAX_LOOPS=3600
#sleep 3
# 设置输出文件路径
OUTPUT_FILE="ping_monitor.log"

# 设置默认探测的主机列表
DEFAULT_HOSTS="172.31.128.1 172.31.128.2 172.31.128.3 172.31.128.4 172.31.128.5 172.31.128.6 172.31.128.7 172.31.128.8 172.31.128.9 172.31.128.10 172.31.128.11 172.31.128.12 172.31.128.13 172.31.128.14 172.31.128.15 172.31.128.16 172.31.128.17 172.31.128.18 172.31.128.19 172.31.128.20"
#DEFAULT_HOSTS="8.8.8.8 1.1.1.1 9.9.9.9 172.31.128.2"
# 如果没有提供参数，使用默认主机
if [ "$#" -lt 1 ]; then
    echo "警告：没有提供主机参数，将使用默认主机列表: $DEFAULT_HOSTS" | tee -a $OUTPUT_FILE
    HOSTS=$DEFAULT_HOSTS
else
    # 获取所有本地IP地址（IPv4）
    LOCAL_IPS=$(ifconfig | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*')

    # 过滤掉本地IP地址
    FILTERED_HOSTS=""
    for host in "$@"; do
        is_local=0
        for ip in $LOCAL_IPS; do
            if [ "$host" = "$ip" ]; then
                is_local=1
                break
            fi
        done

        if [ $is_local -eq 0 ]; then
            FILTERED_HOSTS="$FILTERED_HOSTS $host"
        else
            echo "排除本地IP: $host" | tee -a $OUTPUT_FILE
        fi
    done

    # 检查过滤后是否还有主机
    if [ -z "$FILTERED_HOSTS" ]; then
        echo "错误：所有提供的主机都是本地IP地址，将使用默认主机列表: $DEFAULT_HOSTS" | tee -a $OUTPUT_FILE
        HOSTS=$DEFAULT_HOSTS
    else
        HOSTS=$FILTERED_HOSTS
    fi
fi

NUM_HOSTS=$(echo $HOSTS | wc -w)

# 设置退出时清理
trap 'echo "\n监控结束" | tee -a $OUTPUT_FILE; exit 0' INT TERM

echo "监控的主机: $HOSTS" | tee -a $OUTPUT_FILE

# 添加循环计数器
LOOP_COUNT=0


while [ $LOOP_COUNT -lt $MAX_LOOPS ]; do
    # 使用普通变量存储结果
    current_ms=""

    # 对每个主机进行ping测试
    i=0
    for host in $HOSTS; do
        ping_result=$(ping -c 1 "$host" | grep 'time=' | cut -d '=' -f 4 | cut -d ' ' -f 1 2>/dev/null)

        if [ -z "$ping_result" ]; then
            current_ms="$current_ms -1ms "
        else
            current_ms="$current_ms ${ping_result}ms "
        fi
        i=$((i + 1))
    done

    # 输出结果到屏幕和文件
    echo "$current_ms" | tee -a $OUTPUT_FILE

    # 增加循环计数器
    LOOP_COUNT=$((LOOP_COUNT + 1))

    # 等待1秒
    sleep 1
done


exit 0