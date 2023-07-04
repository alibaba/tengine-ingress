#!/bin/bash

usage()
{
    echo "-add add kernel parameters"
    echo "-check check kernel parameters"
}

kernel()
{
    mount -o remount,rw -t proc /proc/sys /proc/sys

    declare -A params

    params=(
        ["vm.swappiness"]="10"
        ["net.ipv4.tcp_rmem"]="8192 87380 12582912"
        ["net.ipv4.tcp_wmem"]="8192 65536 12582912"
        ["net.ipv4.tcp_mem"]="5242880 7864320 10485760"
        ["kernel.core_pattern"]="/home/coredump/core-%u-%e-%p-%t"
        ["net.ipv4.ip_local_port_range"]="9000 65535"
        ["net.ipv4.tcp_fin_timeout"]="5"
        ["net.ipv4.tcp_tw_timeout"]="3"
        ["net.ipv4.tcp_tw_reuse"]="1"
        ["net.ipv4.tcp_tw_recycle"]="0"
        ["net.ipv4.tcp_timestamps"]="0"
        ["net.ipv4.tcp_retries2"]="6"
        ["net.ipv4.tcp_slow_start_after_idle"]="0"
        ["net.ipv4.tcp_max_orphans"]="1048576"
        ["kernel.printk"]="7 4 1 7"
        ["net.ipv4.tcp_max_syn_backlog"]="65535"
        ["net.core.somaxconn"]="65535"
        ["net.unix.max_dgram_qlen"]="65535"
        ["net.core.netdev_max_backlog"]="200000"
        ["net.unix.max_dgram_qlen"]="65535"
    )

    conf='/etc/sysctl.conf'
    for key in $(echo ${!params[*]})
    do
        value=${params[$key]}
        echo "key=$key,value=$value"
        if grep -q "$key *= " $conf; then   
            sed -i 's/^$key=.*/&value/' $conf
        else
            echo "$key=$value" >>$conf
        fi
    done

    sysctl -p
}

add()
{
    ## setting kernel parameters start ##
    echo `date`": setting kernel parameters start"
    sudo /usr/alisys/dragoon/libexec/armory/bin/safetyout.sh 1
    sudo pkill -INT tsar

    kernel

    sudo mount -o remount,rw -t proc /proc/sys /proc/sys
    sudo sysctl -w vm.swappiness=10
    sudo sysctl -w net.ipv4.tcp_mem="5242880  7864320  10485760"
    sudo sysctl -w net.ipv4.tcp_fin_timeout=5
    sudo sysctl -w net.ipv4.tcp_max_syn_backlog=65535
    sudo sysctl -w net.ipv4.tcp_slow_start_after_idle=0
    sudo sysctl -w net.ipv4.tcp_retries2=6
    sudo sysctl -w net.ipv4.tcp_tw_reuse=1
    sudo sysctl -w net.ipv4.tcp_tw_timeout=3
    #sudo sysctl -w net.ipv4.tcp_rmem="8192  16384 32768"
    #sudo sysctl -w net.ipv4.tcp_wmem="8192  16384 32768"
    # increase Linux autotuning TCP buffer limit to 12MB
    sudo sysctl -w net.ipv4.tcp_rmem="8192 87380 12582912"
    sudo sysctl -w net.ipv4.tcp_wmem="8192 65536 12582912"
    sudo sysctl -w net.ipv4.tcp_tw_timeout=3
    sudo sysctl -w net.ipv4.tcp_tw_recycle=0
    sudo sysctl -w net.ipv4.tcp_timestamps=0
    sudo sysctl -w kernel.core_pattern="/home/coredump/core-%u-%e-%p-%t"
    sudo sysctl -w net.ipv4.ip_local_port_range="9000 65535"
    sudo sysctl -w kernel.printk="7 4 1 7"
    sudo sysctl -w net.core.somaxconn=65535
    sudo sysctl -w net.unix.max_dgram_qlen=65535
    sudo sh -c "echo never > /sys/kernel/mm/transparent_hugepage/enabled"
    sudo sh -c "echo never > /sys/kernel/mm/transparent_hugepage/defrag"

    free_kbytes=`awk '($1 == "MemTotal:"){printf "%d", $2/12-$2/12%4}' /proc/meminfo`
    if [ ${free_kbytes} -gt 400000 ]
    then
        sudo sysctl -w "vm.min_free_kbytes=${free_kbytes}"
    else
        echo `date`": free_kbytes=${free_kbytes} too small, ignore"
    fi

    if [ -f /etc/profile.d/pouchenv.sh ];then
        source /etc/profile.d/pouchenv.sh
    fi

    source /home/admin/base_lib.sh

    echo `date`": setting kernel parameters end"
    ## setting kernel parameters end ##
}

check()
{
    echo `date`": checking kernel parameters start"

    declare -A params

    params=(
        ["vm.swappiness"]="10"
        ["net.ipv4.tcp_rmem"]="8192 87380 12582912"
        ["net.ipv4.tcp_wmem"]="8192 65536 12582912"
        ["net.ipv4.tcp_mem"]="5242880 7864320 10485760"
        ["net.ipv4.ip_local_port_range"]="9000 65535"
        ["net.ipv4.tcp_fin_timeout"]="5"
        ["net.ipv4.tcp_tw_timeout"]="3"
        ["net.ipv4.tcp_tw_reuse"]="1"
        ["net.ipv4.tcp_tw_recycle"]="0"
        ["net.ipv4.tcp_timestamps"]="0"
        ["net.ipv4.tcp_retries2"]="6"
        ["net.ipv4.tcp_slow_start_after_idle"]="0"
        ["net.ipv4.tcp_max_orphans"]="1048576"
        ["kernel.printk"]="7 4 1 7"
        ["net.ipv4.tcp_max_syn_backlog"]="65535"
        ["net.core.somaxconn"]="65535"
        ["net.unix.max_dgram_qlen"]="65535"
        ["net.core.netdev_max_backlog"]="200000"
    )

    kernel_conf='cat /etc/sysctl.conf'
    kernel_mem='sysctl -a'
    for key in $(echo ${!params[*]})
    do
        value=${params[$key]}
        sys_conf_value=`$kernel_conf | grep $key= | tail -n 1 | awk -F '=' '{print $2}'`
        sys_conf_value=`echo "${sys_conf_value}" | xargs echo -n`
        sys_mem_value=`$kernel_mem | grep $key= | tail -n 1 | awk -F ' = ' '{print $2}'`
        sys_mem_value=`echo "${sys_mem_value}" | xargs echo -n`
        if [[ "$sys_conf_value" != "$value" ]];then
            echo `date`": $key is not equal in config file: $value != $sys_conf_value"
            exit 1
        fi

        if [[ "$sys_mem_value" != "$value" ]] && [[ "$sys_mem_value" != "" ]];then
            echo `date`": $key is not equal in memory: $value != $sys_mem_value"
            exit 1
        fi
    done

    echo `date`": checking kernel parameters successfully"
}

#main
while [ "$1" ]; do
    case $1 in
        -add )
            shift
            add
            exit 0
            ;;
        -check )
            shift
            check
            exit 0
            ;;
        * )
            usage [$1]
            exit 1
            ;;
    esac
    shift
done
