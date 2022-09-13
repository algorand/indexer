import atexit
import glob
import logging
import os
import shutil
import tempfile

from e2e_common.util import hassuffix, xrun, firstFromS3Prefix, countblocks, atexitrun
from e2e_conduit.fixtures.plugin_fixture import PluginFixture

logger = logging.getLogger(__name__)


class AlgodImporter(PluginFixture):
    def __init__(self):
        self.algoddir = None
        self.name = "algod"
        self.lastblock = None
        super(AlgodImporter, self).__init__()

    def resolve_config_input(self):
        with open(os.path.join(self.algoddir, 'algod.net'), 'r') as algod_net:
            self.config_input['netaddr'] = "http://" + algod_net.read().strip()
        with open(os.path.join(self.algoddir, 'algod.token'), 'r') as algod_token:
            self.config_input['token'] = algod_token.read().strip()

    def resolve_config_output(self):
        self.config_output['token'] = self.config_input['token']
        self.config_output['netaddr'] = self.config_input['netaddr']
        self.config_output['algoddir'] = self.algoddir

    def setup(self, tempdir, sourcenet):
        source_is_tar = hassuffix(sourcenet, ".tar", ".tar.gz", ".tar.bz2", ".tar.xz")
        if not (source_is_tar or (sourcenet and os.path.isdir(sourcenet))):
            # Assuming sourcenet is an s3 source net
            tarname = f"{sourcenet}.tar.bz2"
            # fetch test data from S3
            bucket = "algorand-testdata"
            import boto3
            from botocore.config import Config
            from botocore import UNSIGNED
            s3 = boto3.client("s3", config=Config(signature_version=UNSIGNED))
            tarpath = os.path.join(tempdir, tarname)
            prefix = "indexer/e2e4"
            success = firstFromS3Prefix(s3, bucket, prefix, tarname, outpath=tarpath)
            if not success:
                raise Exception(
                    f"failed to locate tarname={tarname} from AWS S3 path {bucket}/{prefix}"
                )
            source_is_tar = True
            sourcenet = tarpath
        tempnet = os.path.join(tempdir, "net")
        if source_is_tar:
            xrun(["tar", "-C", tempdir, "-x", "-f", sourcenet])
        else:
            xrun(["rsync", "-a", sourcenet + "/", tempnet + "/"])
        blockfiles = glob.glob(
                os.path.join(tempdir, "net", "Primary", "*", "*.block.sqlite")
                )
        self.lastblock = countblocks(blockfiles[0])
        try:
            xrun(["goal", "network", "start", "-r", tempnet])
        except Exception:
            logger.error("failed to start private network, looking for node.log")
            for root, dirs, files in os.walk(tempnet):
                for f in files:
                    if f == "node.log":
                        p = os.path.join(root, f)
                        logger.error("found node.log: {}".format(p))
                        with open(p) as nf:
                            for line in nf:
                                logger.error("   {}".format(line))
            raise

        # atexitrun(["goal", "network", "stop", "-r", tempnet])
        self.algoddir = os.path.join(tempnet, "Primary")
