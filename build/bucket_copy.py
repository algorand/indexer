# python3 bucket_copy -s $HOME/projects/indexer/tmp/node_pkgs/linux/amd64 -d s3://algorand-staging/indexer/$VERSION

import argparse
import boto3
import glob
import os
import pathlib
import re

def upload_files(globspec, bucket_name, prefix = None):
    response = True
    if isinstance(globspec, str):
        globspecs = (globspec,)
    else:
        globspecs = globspec
    for globspec in globspecs:
        files = glob.glob(globspec, recursive=True)
        for file in files:
            if os.path.isfile(file):
                object_name = os.path.basename(file)
#                if prefix != None:
#                    # Remove both "./" from the beginning and "/" from both sides, if present.
#                    object_name = f"{prefix.lstrip('.').strip('/')}/{object_name}"
                response &= upload_file(file, bucket_name, object_name)
    return response


def upload_file(file_name: str, bucket_name: str, object_name=None) -> bool:
    try:
        if object_name is None:
            object_name = os.path.basename(file)
        s3_client = boto3.client('s3')
        print("uploading file '{}' to bucket '{}' to object '{}'".format(file_name, bucket_name, object_name))
        response = s3_client.upload_file(file_name, bucket_name, object_name)
        if response is not None:
            print(response)
        return True
    except ClientError as e:
        return False

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("-s", "--src", help="The location to copy from", required=True, type=str)
    parser.add_argument("-d", "--dest", help="The location to copy to", required=True, type=str)
    args = parser.parse_args()
    src = args.src
    dest = args.dest

    # https://docs.aws.amazon.com/AmazonS3/latest/dev/BucketRestrictions.html
    bucketRe = re.compile(r'^(s3://)?([a-z0-9-.]*)\/?(.*)?$')

    src_is_remote, src_bucket, src_prefix = bucketRe.findall(src)[0]
    dest_is_remote, dest_bucket, dest_prefix = bucketRe.findall(dest)[0]

    if src_is_remote or dest_is_remote:
        # Uploading local -> s3.
        if os.path.isdir(src):
            # This will handle all cases correctly:
            # 1. foo -> foo/*
            # 2. foo/ -> foo/*
            # 3. foo/* -> foo/*
            src_prefix = src_prefix.rstrip('*').rstrip('/') + '/*'

#            print("src_bucket", src_bucket)
#            print("src_prefix", src_prefix)
#            print("dest_bucket", dest_bucket)
#            print("dest_prefix", dest_prefix)
#            print('/'.join((src_bucket, src_prefix)))

        # To get the wildcard, join the last two tuple elements, i.e., ('', 'bar, '*.out') => 'bar/*.out'
        upload_files('/'.join((src_bucket, src_prefix)), dest_bucket, dest_prefix)
    else:
        raise ValueError('src and dest configs cannot both be local')

if __name__ == '__main__':
    main()


