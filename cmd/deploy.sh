#!/bin/bash

WORK_PATH=$(cd "$(dirname "$0")";pwd)
SUPERVISOR_CONF=$(cat<<EOF
[program:mirror]
autorestart=True
autostart=True
redirect_stderr=True
command=${WORK_PATH}/mirror
user=www
directory=${WORK_PATH}
stdout_logfile_maxbytes = 20MB
stdout_logfile_backups = 20
stdout_logfile = ${WORK_PATH}/supervisor_stdout.log
EOF
)
which supervisord
if [ $? -ne 0 ]; then
    yum -y install supervisor
fi
if [ ! -f /etc/supervisord.d/mirror-supervisor.ini ];then
  echo "${SUPERVISOR_CONF}" > /etc/supervisord.d/mirror-supervisor.ini
fi
systemctl start supervisord
systemctl enable supervisord



