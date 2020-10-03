#!/bin/bash
set -xe
set -o pipefail

CURRENT_DIR=$(cd $(dirname $0);pwd)
export MYSQL_HOST=10.162.28.102
export MYSQL_PORT=3306
export MYSQL_USER=isucon
export MYSQL_DBNAME=xsuportal
export MYSQL_PWD=isucon
export LANG="C.UTF-8"
cd $CURRENT_DIR

cat setup.sql schema.sql | mysql --defaults-file=/dev/null -h $MYSQL_HOST -P $MYSQL_PORT -u $MYSQL_USER $MYSQL_DBNAME