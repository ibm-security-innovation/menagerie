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
docker=/usr/bin/docker
reg_server=localhost:5000

# process command line arguments before all
for i in "$@"
do
case $i in
    -c|--renew-certs)
    renew_certs=TRUE
    shift # past argument
    ;;
    -r=*|--reg-svr=*)
    reg_server="${i#*=}"
    shift # past argument=value
    ;;
    -u=*|--login-user=*)
    login_user="${i#*=}"
    shift # past argument=value
    ;;
    -p=*|--login-pass=*)
    login_pass="${i#*=}"
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
    echo "Usage: bash reg-connect.sh [OPTIONS]"
    echo ""
    echo "  -c,--renew-certs      Do we need to collect certs from registry. Default no, requires su privillages"
    echo "  -r,--reg-svr          Registry server, default localhost:5000"
    echo "  -u,--login-user       Registry user - if provided, will try to login to the registry server"
    echo "  -p,--login-pass       Registry password - f provided, will try to login to the registry server"
    echo "  -h,--help             Display this message"
    exit
fi
echo "renew_certs    = ${renew_certs}"
echo "reg_server     = ${reg_server}"
echo "login_user     = ${login_user}"
echo "login_pass     = ${login_pass}"

if [ $renew_certs ]; then
	  if [ -d /usr/local/share/ca-certificates/ ]; then
	    echo update CA store on ubuntu
	    openssl s_client -connect $reg_server -showcerts </dev/null 2>/dev/null | openssl x509 -outform PEM | tee /usr/local/share/ca-certificates/$reg_server.crt
	    update-ca-certificates
	  elif [ -d /etc/pki/ca-trust/source/anchors/ ]; then
	    echo update CA store on AMI/Centos
	    update-ca-trust force-enable
	    openssl s_client -connect $reg_server -showcerts </dev/null 2>/dev/null | openssl x509 -outform PEM | tee /etc/pki/ca-trust/source/anchors/$reg_server.crt
	    update-ca-trust extract
	  else
	    echo could not detect OS not updating CA
	  fi
	  service docker restart
	  sleep 3
fi

if [ $login_user -a $login_pass ]; then
	echo creds provided, docker login
	$docker login -e 'menagerie@localhost' -u $login_user -p $login_pass $reg_server
fi
