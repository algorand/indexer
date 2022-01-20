#!/usr/bin/python3

import os
import sys
from subprocess import check_output

# Validates that certain files are not modified after running "go generate"

# List of directories that contain generate.go files
list_of_directories=["../idb/postgres/internal/schema"]


first = check_output(["git status --porcelain"], shell=True).strip().decode('UTF-8')

for dir in list_of_directories:
    os.system("/usr/local/go/bin/go generate {}".format(dir))

second = check_output(["git status --porcelain"], shell=True).strip().decode('UTF-8')

if first != second:
    print("Output of 'git status --porcelain':\n {}".format(second))
    sys.exit(1)