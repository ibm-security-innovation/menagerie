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
reg_server="localhost:5000"
latest_tag="latest"

for i in "$@"
do
case $i in
    -r=*|--reg-server=*)
    reg_server="${i#*=}"
    shift # past argument=value
    ;;
    -t|--use-git-tag)
    use_git_tag=TRUE
    shift # past argument with no value
    ;;
    -s|--run-sanity)
    run_sanity=TRUE
    shift # past argument with no value
    ;;
    -m|--restart-menagerie)
    restart_menagerie=TRUE
    shift # past argument with no value
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
    echo "  -m,--restart-menagerie Whether to restart the menagerie service"
    echo "  -t,--use_git_tag       Use the latest tag from git for docker container. Default is FALSE will result in using tag 'latest'."
    echo "  -s,--run-sanity        Run the sanity testing after build"
    echo "  -h,--help              Display this message"
    exit
fi
echo "reg_server        = ${reg_server}"
echo "restart_menagerie = ${restart_menagerie}"
echo "use_git_tag       = ${use_git_tag}"
echo "run_sanity        = ${run_sanity}"

set -ex
if [ $use_git_tag ]; then
  latest_tag=`git describe --abbrev=0 --tags | sed 's/\//_/'`
fi
echo "using $latest_tag for docker image\n"

# build the latest binaries
export GO15VENDOREXPERIMENT=1
export GOPATH=$PWD
go install ./src/cmd/...

# pack and send container
if [ $reg_server ]; then
  docker build -t $reg_server/menagerie .
  docker tag $reg_server/menagerie $reg_server/menagerie:$latest_tag
  docker push $reg_server/menagerie:$latest_tag
fi

if [ $restart_menagerie ]; then
  stop menagerie
  start menagerie
fi

if [ $run_sanity ]; then
  go list ./src/... | grep -v vendor | sed -e 's/^/.\/src\//' | xargs go test
  ./src/test/test.sh "-r=$reg_server"
fi
