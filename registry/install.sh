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

targetdir=/data/menagerie
volumedir=$targetdir/volumes/registry
docker=`which docker`
docker_compose=`which docker-compose`
if [ ! -f $volumedir/auth/cred ]; then
  echo first time install detected
  firsttime=1
fi
stop menagerie_reg

# create directories
mkdir -p $volumedir/auth
mkdir -p $volumedir/certs

# copy files and move to location
cp ./registry-compose.yml $targetdir
cp ./menagerie_reg.conf /etc/init/

if [ $firsttime ]; then
  # create self signed certificate
  subject="/O=menagerie/commonName=localhost/organizationalUnitName=menagerie/emailAddress=menagerie@localhost"
  openssl req -newkey rsa:4096 -nodes -sha256 -keyout $volumedir/certs/domain.key -subj $subject -x509 -days 365 -out $volumedir/certs/domain.crt

  # create credentials, clear text
  echo first time, creating cred and cert files
  echo `date | md5sum | head -c 16` > $volumedir/auth/cred && sleep 1 && echo `date | md5sum | head -c 32` >> $volumedir/auth/cred
  pushd $volumedir
  $docker run --rm --entrypoint=/usr/bin/htpasswd registry:2 -Bbn `head -1 auth/cred` `tail -1 auth/cred` >> auth/htpasswd
  popd
fi

$docker_compose -f $targetdir/registry-compose.yml -p mngreg pull
$docker network create mngreg_default
$docker_compose -f $targetdir/registry-compose.yml -p mngreg create
tar c -C $volumedir . | $docker run --rm -v mngreg_registry_conf:/data -i busybox tar x -C /data
start menagerie_reg
