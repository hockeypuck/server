#!/bin/bash -ex

killall hockeypuck
rm -rf ptree-peer*

go build

mongo <<EOF
use peer1;
db.dropDatabase();
use peer2;
db.dropDatabase();
EOF

./hockeypuck -config sample-peer1.conf &
./hockeypuck -config sample-peer2.conf &
