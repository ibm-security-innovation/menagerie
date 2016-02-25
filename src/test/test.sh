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

cd `dirname $0`

reg_server=localhost:5000

# process command line arguments before all
for i in "$@"
do
case $i in
    -r=*|--reg-server=*)
    reg_server="${i#*=}"
    shift # past argument=value
    ;;
    -n|--no-cleanup)
    nocleanup=TRUE
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
    echo "Usage: bash -x run.sh [OPTIONS]"
    echo ""
    echo "  -r,--reg-server    Registry server, default localhost:5000"
    echo "  -n,--no-cleanup    Don't run cleanup after the test (for debugging)"
    echo "  -h,--help          Display this message"
    exit
fi
# TODO run with specific uid
uid=0
targetdir=/tmp/menagerie/test
volumedir=$targetdir/volumes
basedir=../..
docker=`which docker`
docker_compose=`which docker-compose`

pwd
echo "reg_server     = ${reg_server}"
echo "user           = ${uid}"

# if [ -n "$(docker ps -q)" ] ; then
#   echo "ERROR: There are already running containers. Exiting."
#   exit -1
# fi

# create volumes
function create_volume() {
  vol=$volumedir/$1
  mkdir -p $vol/log
  mkdir -p $vol/keys
}

function template() {
  file=$1
  name=$2
  val=$3
  sed -i "s|{{$name}}|$val|g" $file # NOTE this fails if $val contains '|'
}

mkdir -p $volumedir/mysql
mkdir -p $volumedir/rabbitmq
mkdir -p $volumedir/test
create_volume frontend
mkdir -p $volumedir/frontend/store
create_volume menage

# copy files and move to location
cp $basedir/deploy/sources/menagerie-compose.yml $volumedir/test/test-compose.yml
cp $basedir/src/test/confs/default.json $volumedir/frontend/keys/frontend.json
cp $basedir/src/test/confs/engines.json $volumedir/frontend/keys/engines.json
cp $basedir/src/test/confs/default.json $volumedir/menage/keys/menage.json
cp $basedir/src/test/confs/engines.json $volumedir/menage/keys/engines.json
cp -r ~/.docker $volumedir/menage/keys/
cp -R $basedir/deploy/sources/sql $volumedir/mysql/

# run parameters through template files
template $volumedir/test/test-compose.yml 'home' $HOME
template $volumedir/test/test-compose.yml 'regserver' $reg_server
template $volumedir/test/test-compose.yml 'uid' $uid
template $volumedir/test/test-compose.yml 'port' 8080
template $volumedir/menage/keys/engines.json 'regserver' $reg_server
template $volumedir/menage/keys/engines.json 'uid' $uid
template $volumedir/frontend/keys/engines.json 'regserver' $reg_server
template $volumedir/frontend/keys/engines.json 'uid' $uid

# change ownership for frontend and engine volumes
chown -R $user:$user $volumedir/frontend
chmod -R g+s $volumedir/frontend

# initialize containers
$docker_compose -f $volumedir/test/test-compose.yml create
tar c -C $volumedir/frontend . | $docker run --rm -v test_frontend:/data -i busybox tar x -C /data
tar c -C $volumedir/menage . | $docker run --rm -v test_menage:/data -i busybox tar x -C /data

# start menagerie
$docker_compose -f $volumedir/test/test-compose.yml up -d

# Apply schema
stat="1"
for i in {1..30}; do 
  sleep 2
  echo $(date +%T) "Waiting to configure database..."
  $docker exec -i test_mysql_1 mysql -umenagerie -pmenagerie < $basedir/deploy/sources/sql/schema.sql > /dev/null 2>&1
  stat="$?"
  if [ $stat -eq "0" ]; then
    break
  fi
done

if [ $stat -eq "0" ]; then
  echo "Connected to database successfully"
  export GOPATH=$(readlink -m $basedir/../src)  # TODO - make generic
  sleep 3
  go test -tags integration $basedir/src/test
  exitcode="$?"
else
  echo "Couldn't connect to database"
fi

if [ -z $nocleanup ]; then
  docker-compose -f $volumedir/test/test-compose.yml stop
  docker-compose -f $volumedir/test/test-compose.yml rm -f -v
  sudo rm -rf $targetdir
fi

exit $exitcode
