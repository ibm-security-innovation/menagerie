#!/bin/bash
# 
#  Copyright 2015 IBM Corp. All Rights Reserved.
# 
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
# 
#       http://www.apache.org/licenses/LICENSE-2.0
# 
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
# 

reg_server=localhost:5000
deploy_env=generic-ami
mule_server=

# process command line arguments before all
for i in "$@"
do
case $i in
    -s|--restart-shared)
    restart_shared=TRUE
    shift # past argument
    ;;
    -r=*|--reg-server=*)
    reg_server="${i#*=}"
    shift # past argument=value
    ;;
    -m=*|--mule-server=*)
    mule_server="${i#*=}"
    shift # past argument=value
    ;;
    -e=*|--deploy-env=*)
    deploy_env="${i#*=}"
    shift # past argument=value
    ;;
    -u=*|--user=*)
    user="${i#*=}"
    shift # past argument=value
    ;;
    -h|--help)
    helpmsg=TRUE
    shift # past argument with no value
    ;;
    *)
            # unknown option
    ;;
esac
done
if [ $helpmsg ]; then
    echo "Usage: bash -x install.h [OPTIONS]. It is assumed the user is logged in into the remote/local registry"
    echo ""
    echo "  -r,--reg-server        Registry server, default localhost:5000"
    echo "  -e,--deploy-env	   Deploy environment, default generic-ami"
    echo "  -m,--mule-server       Mule server, default empty"
    echo "  -u,--user              User for running frontend and engines"
    echo "  -s,--restart-shared    Whether to restart the base DB/MQ services. Default no."
    echo "  -h,--help              Display this message"
    exit
fi
echo "restart_shared = ${restart_shared}"
echo "reg_server     = ${reg_server}"
echo "mule_server    = ${mule_server}"
echo "user           = ${user}"

uid=`id -u $user`
targetdir=/data/menagerie
volumedir=$targetdir/volumes
docker=`which docker`
docker_compose=`which docker-compose`
if [ ! -e /var/opt/menagerie ]; then
  ln -s $targetdir /var/opt/menagerie
fi

# stop previous
stop menagerie
if [ $restart_shared ]; then
  echo will restart db and mq as well
  sleep 3
  stop menagerie_db
  stop menagerie_mq
fi


# create volumes
function create_volume() {
  vol=$volumedir/$1
  mkdir -p $vol/log
  mkdir -p $vol/keys
  mkdir -p $vol/mule/incoming
  mkdir -p $vol/mule/failed
  mkdir -p $vol/mule/processed
}

function template() {
  file=$1
  name=$2
  val=$3
  sed -i "s|{{$name}}|$val|g" $file # NOTE this fails if $val contains '|'
}

mkdir -p $targetdir/scripts
mkdir -p $volumedir/mysql
mkdir -p $volumedir/rabbitmq
create_volume frontend
mkdir -p $volumedir/frontend/store
create_volume menage

# copy files and move to location
# > docker-compose.yaml,menagerie.conf come from RPM
cp ./sources/menagerie-compose.yml $volumedir/menage
cp ./sources/db-compose.yml $volumedir/mysql
cp ./sources/mq-compose.yml $volumedir/rabbitmq
cp ./sources/cleanup.sh $targetdir/scripts
cp ./sources/job_mule_add.sh $targetdir/scripts
cp ../environments/$deploy_env/confs/default.json $volumedir/frontend/keys/frontend.json
cp ../environments/$deploy_env/confs/engines.json $volumedir/frontend/keys/engines.json
cp ../environments/$deploy_env/confs/default.json $volumedir/menage/keys/menage.json
cp ../environments/$deploy_env/confs/engines.json $volumedir/menage/keys/engines.json
cp ./sources/log.sh $volumedir/menage/
cp -R ./sources/sql $volumedir/mysql/
cp ./sources/menagerie.conf /etc/init/
cp ./sources/menagerie_db.conf /etc/init/
cp ./sources/menagerie_mq.conf /etc/init/
cp ./sources/menagerie.logrotate /etc/logrotate.d/
cp ./sources/menagerie.crontab /etc/cron.d/

# run parameters through template files
if [ $mule_server ]; then
  mule_cmd="-u $mule_server"
fi
template /etc/cron.d/menagerie.crontab 'muleserver' $mule_cmd
template $volumedir/menage/menagerie-compose.yml 'home' $HOME
template $volumedir/menage/menagerie-compose.yml 'regserver' $reg_server
template $volumedir/menage/menagerie-compose.yml 'uid' $uid
template $volumedir/menage/keys/engines.json 'regserver' $reg_server
template $volumedir/menage/keys/engines.json 'uid' $uid
template $volumedir/frontend/keys/engines.json 'regserver' $reg_server
template $volumedir/frontend/keys/engines.json 'uid' $uid

# change ownership for frontend and engine volumes
chown -R $user:$user $volumedir/frontend
chmod -R g+s $volumedir/frontend

# hide the rest
chmod -R 700 $volumedir/menage
chmod -R 700 $volumedir/mysql
chmod -R 700 $volumedir/rabbitmq
chmod -R 700 $targetdir/scripts

# start shared if requested
if [ $restart_shared ]; then
  $docker_compose -f $volumedir/mysql/db-compose.yml pull
  $docker_compose -f $volumedir/rabbitmq/mq-compose.yml pull
  start menagerie_db
  timer="1"
  stat="1"
  while [ "$stat" -ne "0" ] && [ "$timer" -le "32" ]; do 
    sleep $timer
    timer=$[$timer+$timer]
    $docker exec -i menagerie_mysql mysql -umenagerie -pmenagerie < ./sources/sql/schema.sql
    stat="$?"
  done
  start menagerie_mq
  sleep 3
fi

# start menagerie
$docker_compose -f $volumedir/menage/menagerie-compose.yml pull
start menagerie
