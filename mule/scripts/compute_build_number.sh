#!/usr/bin/env bash

if [ -n "$VERSION" ]
then
    echo "$VERSION"
else
    cat .version
fi

