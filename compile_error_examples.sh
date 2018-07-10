#!/bin/bash

go build
if [[ $? -ne 0 ]]; then
    exit 1
fi
for file in $(find -wholename './examples/errors/*.al'); do
    echo "Compiling: $file"
    ./alang $file
    echo
done
exit 0
