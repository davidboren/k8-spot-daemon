#!/bin/bash

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

eval $(aws ecr get-login --registry-ids 155537346795 --region us-west-2)

cd $DIR/../

docker build -t k8-spot-daemon ./

REPOSITORY="155537346795.dkr.ecr.us-west-2.amazonaws.com/k8-spot-daemon"

docker tag conductor $REPOSITORY:latest
docker push $REPOSITORY:latest
