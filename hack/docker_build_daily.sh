#!/bin/bash

echo $TRAVIS_EVENT_TYPE
echo $TRAVIS_BRANCH
echo "building container"

if [[ "$TRAVIS_EVENT_TYPE" = "cron" && "$TRAVIS_BRANCH" = "master" ]]; then
    echo "$DOCKER_PASSWORD" | docker login -u "$DOCKER_USERNAME" --password-stdin;
    make docker-build;
    make docker-push;
fi