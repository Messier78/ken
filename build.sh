#! /bin/bash

ChangeLog="rtmp server"
Version="1.0.1"
BuildTime=$(date +'%Y.%m.%d %H:%M:%S')

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${DIR}"
mkdir -p bin/

LDFLAGS="
  -X 'ken/command.Built=${BuildTime}'
  -X 'ken/command.Version=${Version}'
  -X 'ken/command.ChangeLog=${ChangeLog}'
  -s
  -w
"

echo "build ken ..."
go build -ldflags "${LDFLAGS}" -o "${DIR}/bin/ken" .

if [ a"$1" = "arun" ]; then
  ${DIR}/bin/ken server start
fi

if [ a"$1" = "alinux" ]; then
  GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o "${DIR}/bin/ken" .
  rm /usr/local/share/nginx/build/ken
  mv ${DIR}/bin/ken /usr/local/share/nginx/build/
fi