# Configuration

Configuration is stored in a file in the data directory named `conduit.yml`.
Use `./conduit -h` for command options.

## conduit.yml

There are several top level configurations for configuring behavior of the conduit process. Most detailed configuration is made on a per-plugin basis. These are split between `Importer`, `Processor` and `Exporter` plugins.

Here is an example configuration which shows the general format:
```yaml
# optional: if present perform runtime profiling and put results in this file.
cpu-profile: "path to cpu profile file."

# optional: maintain a pidfile for the life of the conduit process.
pid-filepath: "path to pid file."

# optional: level to use for logging.
log-level: "INFO, WARN, ERROR"

# Define one importer.
importer:
    name:
    config:

# Define one or more processors.
processors:
  - name:
    config:
  - name:
    config:

# Define one exporter.
exporter:
    name:
    config
```

## Plugin configuration

See [plugins/home.md](plugins/home.md) for details.
Each plugin is identified by a `name`, and provided the `config` during initialization.
