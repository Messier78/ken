#! /bin/bash

ChangeLog="TODO: flv/hls, conf"
Version="1.0.3"
BuildTime=$(date +'%Y.%m.%d %H:%M:%S')
subVersion=$(cat .version)
((subVersion++))
echo $subVersion >.version

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${DIR}"
mkdir -p bin/

LDFLAGS="
  -X 'ken/command.Built=${BuildTime}'
  -X 'ken/command.Version=${Version}.${subVersion}'
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
