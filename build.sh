#! /bin/bash

ChangeLog="first commit"
Version="1.0.0"
BuildTime=`date +'%Y.%m.%d %H:%M:%S'`

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${DIR}"
mkdir -p bin/

LDFLAGS=" \
  -X 'ken/command.BuildTime=${BuildTime}' \
  -X 'ken/command.Version=${Version}'     \
  -X 'ken/command.ChangeLog=${ChangeLog}' \
"

echo "build ken ..."
go build -ldflags "${LDFLAGS}" -o "${DIR}/bin/ken" .