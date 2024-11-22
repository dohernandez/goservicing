#!/usr/bin/env sh

# This code is provided by github.com/dohernandez/dev.

FLAT_TYPES_DEFAULT="major release minor feature feat patch issue hotfix dependabot whitesource/"
FLAT_SEPARATORS_DEFAULT="- /"

types="${1}"

FLAT_TYPES=""

for target in $types
do
  FLAT_TYPES="$FLAT_TYPES $target"
done

# removing the first space added when FLAT_TYPES is empty
FLAT_TYPES=$(echo "${FLAT_TYPES}" | sed 's/^ //g')

# If FLAT_TYPES is empty, use the value of FLAT_TYPES_DEFAULT instead
FLAT_TYPES=${FLAT_TYPES:-$FLAT_TYPES_DEFAULT}

REGEX_FLAT_TYPES=$(echo "${FLAT_TYPES}" | sed 's/ /|/g')


separators="${2}"

FLAT_SEPARATORS=""

for target in $separators
do
  FLAT_SEPARATORS="$FLAT_SEPARATORS $target"
done

# If FLAT_SEPARATORS is empty, use the value of FLAT_TYPES_DEFAULT instead
FLAT_SEPARATORS=${FLAT_SEPARATORS:-$FLAT_SEPARATORS_DEFAULT}

REGEX_FLAT_SEPARATORS=$(echo "${FLAT_SEPARATORS}" | sed 's/ /|/g')



# ^minor([-\/]+.+)?$
if ! (echo "${GITHUB_HEAD_REF}" | grep -i -E "^($REGEX_FLAT_TYPES)([$REGEX_FLAT_SEPARATORS]+.+)?$"); then
    VALID_FLAT_TYPES=$(echo "${FLAT_TYPES}" | sed 's/ /, /g')
    VALID_FLAT_SEPARATORS=$(echo "${FLAT_SEPARATORS}" | sed 's/ /, /g')
    echo "Invalid branch name \"${GITHUB_HEAD_REF}\". Valid branch prefixes: ${VALID_FLAT_TYPES} and separators: ${VALID_FLAT_SEPARATORS}"
    exit 1
fi
echo "Valid branch name"