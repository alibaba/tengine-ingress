#!/bin/bash

APP_NAME=ingress

CHECK_PORT=10254
if [ $2 -a "$2" -gt 0 ] 2>/dev/null; then
    CHECK_PORT=$2
fi

CURL_BIN=/usr/bin/curl
SPACE_STR="..................................................................................................."

OUTIF=`/sbin/route -n | tail -1  | sed -e 's/.* \([^ ]*$\)/\1/'`
HTTP_IP="http://`/sbin/ifconfig | grep -A1 ${OUTIF} | grep inet | awk '{print $2}' | sed 's/addr://g'`:${CHECK_PORT}"

#####################################
checkpage() {
    print_port

    check_port "/healthz" "${APP_NAME}" ""
    if [ $? -eq 0 ]; then
        echo ${SPACE_STR}
        echo "INFO: ${APP_NAME} check status [  OK  ]"
        echo ${SPACE_STR}
        status=1
        return 0
    fi
    echo ${SPACE_STR}
    echo "INFO: ${APP_NAME} check status [FAILED]"
    echo ${SPACE_STR}
    return 1
}

check_page_loop(){
    print_port

    # 5min
    local SLEEP=300
    local exptime=0

    while [ ${SLEEP} -ge 0 ]; do
        check_port "/healthz" "${APP_NAME}" "" "no"
        if [ $? -gt 0 ]; then
            # 还在 preload
            if [ ${SLEEP} -gt 0 ]; then
                sleep 1
            fi
            SLEEP=`expr $SLEEP - 1`
            ((exptime++))
            echo -n -e "\r check $exptime..."
        else
            return 0
        fi
    done

    return 1

}

print_port(){
    echo "INFO: try to check ${APP_NAME} port: ${CHECK_PORT}"
    echo "$CURL_BIN" "${HTTP_IP}${URL}"
}

check_port() {
    # check port
    portret=`(/usr/sbin/ss -ln4 sport = :${CHECK_PORT}; /usr/sbin/ss -ln6 sport = :${CHECK_PORT}) | grep -c ":${CHECK_PORT}"`
    if [ $portret -ne 0 ]; then

        URL=$1
        TITLE=$2
        CHECK_TXT=$3
        NO_ECHO=$4

        if [ "$TITLE" == "" ]; then
            TITLE=$URL
        fi
        if [ "$NO_ECHO" == "" ]; then
            len=`echo $TITLE | wc -c`
            len=`expr 60 - $len`
            echo -n -e "$TITLE ...${SPACE_STR:1:$len}"
        fi
        TMP_FILE=`$CURL_BIN --silent -m 150 "${HTTP_IP}${URL}" 2>&1`

        if [ "$NO_ECHO" == "" ]; then
            echo "$CURL_BIN" "${HTTP_IP}${URL}" " return: "
            echo "$TMP_FILE"
        fi

        if [ "$CHECK_TXT" != "" ]; then
            checkret=`echo "$TMP_FILE" | fgrep "$CHECK_TXT"`
            if [ "$NO_ECHO" == "" ]; then
                echo "check ret:$checkret"
            fi
            if [ "$checkret" == "" ]; then
                if [ "$NO_ECHO" == "" ]; then
                    echo "ERROR: Please make sure $URL return: $CHECK_TXT"
                fi
                error=1
            else
                error=0
            fi
        fi

        return $error
    fi

    #just error
    return 1
}

#####################################

ACTION=$1

main() {
    case "$ACTION" in
        once)
            checkpage
        ;;
        *)
            check_page_loop
        ;;
    esac
}

main

