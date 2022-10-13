# Configuration

Configuration is stored in a file in the data directory named `conduit.yml`.
Use `./conduit -h` for command options.

## conduit.yml

There are several top level configurations for configuring behavior of the conduit process. Most detailed configuration is made on a per-plugin basis. These are split between `Importer`, `Processor` and `Exporter` plugins.

Here is an example configuration which shows the general format:
```yaml
# optional: if present perform runtime profiling and put results in this file.
CPUProfile: "path to cpu profile file."

# optional: maintain a pidfile for the life of the conduit process.
PIDFilePath: "path to pid file."

# optional: level to use for logging.
LogLevel: "INFO, WARN, ERROR"

# Define one importer.
Importer:
    Name:
    Config:

# Define one or more processors.
Processors:
  - Name:
    Config:
  - Name:
    Config:

# Define one exporter.
Exporter:
    Name:
    Config
```

## Plugin configuration

See [plugins/home.md](plugins/home.md) for details.
Each plugin is identified by a `Name`, and provided the `Config` during initialization.
