from datetime import datetime, timedelta
import gzip
import io
import logging
import re

logger = logging.getLogger(__name__)


class subslurp:
    """accumulate stdout or stderr from a subprocess and hold it for debugging if something goes wrong"""
    def __init__(self, f):
        self.f = f
        self.buf = io.BytesIO()
        self.gz = gzip.open(self.buf, "wb")
        self.timeout = timedelta(seconds=120)
        # Matches conduit log output: "Pipeline round: 110"
        self.round_re = re.compile(b'.*"Pipeline round: ([0-9]+)"')
        self.round = 0
        self.error_log = None

    def logIsError(self, log_line):
        if b"error" in log_line:
            self.error_log = log_line
            return True
        return False

    def tryParseRound(self, log_line):
        m = self.round_re.match(log_line)
        if m is not None and m.group(1) is not None:
            self.round = int(m.group(1))

    def run(self, lastround):
        if len(self.f.peek().strip()) == 0:
            logger.info("No Conduit output found")
            return

        start = datetime.now()
        lastlog = datetime.now()
        while (
            datetime.now() - start < self.timeout
            and datetime.now() - lastlog < timedelta(seconds=15)
        ):
            for line in self.f:
                lastlog = datetime.now()
                if self.gz is not None:
                    self.gz.write(line)
                self.tryParseRound(line)
                if self.round >= lastround:
                    logger.info(f"Conduit reached desired lastround: {lastround}")
                    return
                if self.logIsError(line):
                    raise RuntimeError(f"E2E tests logged an error: {self.error_log}")

    def dump(self):
        self.gz.close()
        self.gz = None
        self.buf.seek(0)
        r = gzip.open(self.buf, "rt")
        return r.read()
