#!/bin/bash

# Copyright 2020 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

if [ -n "$DEBUG" ]; then
	set -x
fi

set -o errexit
set -o nounset
set -o pipefail

URL="https://github.com/kubernetes/ingress-nginx/tree/master/"
DIR=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)

echo "

# e2e test suite for [NGINX Ingress Controller]($URL)

"

for FILE in `find $DIR/test/e2e -name "*.go"`;do
    # describe definition
    DESCRIBE=$(cat $FILE | grep -n -oP 'Describe.*')
    # line number
    DESCRIBE_LINE=$(echo $DESCRIBE | cut -f1 -d ':')
    # clean describe, extracting the string
    DESCRIBE=$(echo $DESCRIBE | sed -En 's/.*"(.*)".*/\1/p')

    FILE_URL=$(echo $FILE | sed "s|${DIR}/|${URL}|g")
    echo "
### [$DESCRIBE]($FILE_URL#L$DESCRIBE_LINE)
"

    # extract Tests
    ITS=$(cat $FILE | grep -n -oP 'It\(.*')
    while IFS= read -r line; do
        IT_LINE=$(echo $line | cut -f1 -d ':')
        IT=$(echo $line | sed -En 's/.*"(.*)".*/\1/p')
        echo "- [$IT]($FILE_URL#L$IT_LINE)"
    done <<< "$ITS"
done
