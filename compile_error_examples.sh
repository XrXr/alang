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

for file in $(find -wholename './examples/typecheck/*.al'); do
    echo "Compiling: $file"
    ./alang $file > /dev/null
    echo
done

exit 0
