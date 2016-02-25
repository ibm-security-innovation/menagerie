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

function log() {
    echo "($$) $*"
}


set -u

while getopts "hdu:v:" OPTION
do
    case $OPTION in
        d)
            dryrun=1
            log "dryrun"
            ;;
        u)
            update_url=$OPTARG
            ;;
        h)
            usage
            exit 1
            ;;
        ?)
            usage
            exit 1
            ;;
    esac
done

basedir=/data
logfile=$basedir/log/mule_add.log
incomingdir=$basedir/mule/incoming
faileddir=$basedir/mule/failed
processeddir=$basedir/mule/processed
now=`date +'%Y%m%d-%H%M%S'`
today=`date +'%Y%m%d'`
dayhourslash=`date +'%Y/%m/%d/%H'`

log "Start"
if [ -z ${update_url+x} ]; then log "url is not set"; else log "using url: $update_url"; fi
log "incoming dir: $incomingdir"

# first collect all the relevant files
in_work=$incomingdir/$now.pid-$$.in_work
mkdir $in_work
# we need this xargs usage to handle a large number of input files
find $incomingdir -maxdepth 1 -type f -name "*.mule" | xargs -r mv -t $in_work
# check if the work dir is empty by trying to delete it
rmdir $in_work &> /dev/null
if [[ $? == 0 ]]; then
  log "Done, no mule files"
  exit 0;
fi

combined=$in_work/mule-$now.merged
for i in $in_work/*.mule; do
    cat $i <(printf "\n") >> $combined.tmp
done
sort $combined.tmp | grep -v "^$" > $combined
rm $combined.tmp

# now run mule on all of them combined
if [ -z ${dryrun+x} ]; then
    if [ "${update_url:-}" ]; then
      /usr/bin/curl -s --data-binary @$combined $update_url
    else
      log "empty update url"
    fi

    # move to processed/failed dir
    if [ $? == 0 ] ; then
        mkdir -p $processeddir/$dayhourslash
        mv $combined $processeddir/$dayhourslash/
        find $in_work -type f | xargs rm
        rmdir $in_work
    else
        log "failed to process, exitcode: $?"
        mv $combined $faileddir/
    fi
else
    log "combined file: $combined"
fi

log "Done"
