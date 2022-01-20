#!/usr/bin/python3

import os
import sys
from subprocess import check_output

# Validates that certain files are not modified after running "go generate"

# List of directories that contain generate.go files
list_of_directories=["../idb/postgres/internal/schema"]

for dir in list_of_directories:
    os.system("/usr/local/go/bin/go generate {}".format(dir))

out = check_output(["git status --porcelain | wc -l"], shell=True).strip().decode('UTF-8')

if out != "0":
    print("Number of files modified was not 0.  It was {}".format(out))
    out = check_output(["git status --porcelain"], shell=True).strip().decode('UTF-8')
    print("Output of 'git status --porcelain':\n {}".format(out))
    out = check_output(["git diff"], shell=True).strip().decode('UTF-8')
    print("Output of diff: {}\n".format(out))
    sys.exit(1)