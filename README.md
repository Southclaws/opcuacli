# opcuacli

A terminal client for OPC UA servers: discovery, address space browsing, reads and writes, method calls, history, type and metadata inspection, live subscriptions, and a full-screen explorer.

```sh
opcuacli discover endpoints -e opc.tcp://plant:4840   # what does it offer?
opcuacli browse --depth 2 --values                    # what is in there?
opcuacli monitor 'ns=2;s=Kettle/Temp'                 # what is it doing now?
opcuacli tui                                          # all of the above, interactively
```

## Install

```sh
go install github.com/Southclaws/opcuacli@latest
```

## Connecting

Connection settings come from a profile in a configuration file, and any of them can be overridden by a flag for a one-off check.

```sh
opcuacli config init                     # interactive: asks the server what it offers
opcuacli config list
opcuacli config path
opcuacli ping --profile plant
```

`opcuacli config path` prints where the file lives: the per-user configuration
directory the platform defines.

| Platform | Path                                                                   |
| -------- | ---------------------------------------------------------------------- |
| Windows  | `%AppData%/opcua/config.yaml`                                          |
| macOS    | `~/Library/Application Support/opcua/config.yaml`                      |
| Linux    | `$XDG_CONFIG_HOME/opcua/config.yaml`, or `~/.config/opcua/config.yaml` |

`--config` or `OPCUA_CONFIG` points at any other file. A profile looks like this;
[`examples/config.yaml`](./examples/config.yaml) has a worked set covering a
secured PLC, a redundant pair and a bench simulator:

```yaml
default: plant
profiles:
  plant:
    endpoints: # tried in order: this is the failover set
      - opc.tcp://plant-a:4840
      - opc.tcp://plant-b:4840
    security:
      policy: auto # auto takes the strongest on offer
      mode: auto
    auth:
      mode: username
      username: operator
      passwordFile: C:/ProgramData/opcua/plant.password
    session:
      timeout: 20m
      requestTimeout: 10s
    subscription:
      interval: 500ms
```

Every setting has a flag: `--endpoint`, `--policy`, `--mode`, `--auth`,
`--username`, `--password-file`, `--cert`, `--key`, `--timeout` and so on, plus
`OPCUA_ENDPOINT`, `OPCUA_PROFILE`, `OPCUA_PASSWORD` and friends in the
environment:

```powershell
$env:OPCUA_PASSWORD = Read-Host -AsSecureString | ConvertFrom-SecureString -AsPlainText
opcuacli read 'ns=2;s=Temp' --profile line-1
```

A password written into the file is stored in plain text, and on Windows the
file's mode cannot keep another account out of it, so `auth.passwordFile` or
`OPCUA_PASSWORD` is the better place for one.

## Commands

| Area          | Commands                                                                                |
| ------------- | --------------------------------------------------------------------------------------- |
| Discovery     | `discover endpoints`, `discover servers`, `discover network`                            |
| Address space | `browse`, `find`, `resolve`, `references`                                               |
| Values        | `read`, `write`, `call`                                                                 |
| History       | `history read`, `history at`, `history events`                                          |
| Metadata      | `attributes`, `types get`, `types of`, `types list`, `namespaces`                       |
| Server        | `server info`, `server capabilities`, `server diagnostics`, `server redundancy`, `ping` |
| Live          | `monitor`, `events`                                                                     |
| Explorer      | `tui`                                                                                   |
| Setup         | `config ...`, `cert ...`                                                                |

### Reading and writing

```sh
opcuacli read 'ns=2;s=Temp' 'ns=2;s=Press'
opcuacli read 'ns=2;s=Temp' --attribute Value,DataType,AccessLevel
opcuacli write 'ns=2;s=Setpoint' 21.5          # the type comes from the node
opcuacli write 'ns=2;s=Window' 1,2,3 --array --type int32
opcuacli call 'ns=2;s=Boiler' SetTemperature --describe
```

A write reads the node's `DataType` first and parses the value into it, because
a server rejects a write whose variant type does not match. `--type` overrides
that; `--dry-run` shows what would happen.

### Browsing and searching

```sh
opcuacli browse                                  # the Objects folder, as a tree
opcuacli browse i=2253 --depth 3
opcuacli browse --depth 0 --values --class variable
opcuacli find TEMP --class variable --values
opcuacli resolve Server/ServerStatus/CurrentTime
opcuacli references 'ns=2;s=Machine' --ref-type all
```

### Live values

```sh
opcuacli monitor 'ns=2;s=Temp' 'ns=2;s=Press'    # a live table with sparklines
opcuacli monitor 'ns=2;s=Flow' --deadband absolute --deadband-value 0.1
opcuacli monitor 'ns=2;s=Temp' --format jsonl | tee log.jsonl
opcuacli events --severity-min 500
```

`monitor` labels each row with the node's browse name, keeps one row per node
updated in place on a terminal, and appends frames when redirected. Deadbands,
triggers, queue sizes, monitoring modes, sampling intervals, priorities and
lifetime counts are all exposed.

### Piping

```sh
opcuacli read i=2258 --format json | jq -r '.[0].value'
opcuacli find --class variable temperature --format plain | opcuacli monitor --stdin
opcuacli browse --depth 0 --format jsonl > address-space.jsonl
opcuacli history read 'ns=2;s=Temp' --last 24h --format csv
```

`--format plain` puts the node id first so the first field pipes onward; `json`
and `yaml` follow the schemas in the specification; `jsonl` streams as the walk
proceeds rather than buffering.

Exit codes distinguish failures: `2` no endpoint reachable, `3` the server
returned a bad status, `4` a node, path or profile does not exist.

## Explorer

```sh
opcuacli tui
opcuacli tui --node 'ns=2;s=Machine' --theme nord --watch 'ns=2;s=Temp'
```

Two panes: an expandable tree of the address space on the left, the selected
node on the right. A level is fetched once and cached, so collapsing and
re-expanding costs nothing.

- `↑`/`↓` or `j`/`k` move; `→`/`l` expands a branch in place and `←`/`h`
  collapses it (or steps out to the parent), so the topology builds up as you
  walk it
- `enter` re-roots the tree at the selected node, `esc` goes back up, `~` returns
  to the Objects folder
- `/` filters the current level; `:` jumps to a node id, a standard name, or a
  browse path
- `tab` moves the keyboard to the detail pane, where `←`/`→` change tab
  (attributes, type, references, watch) and `↑`/`↓` scroll
- `w` watches the selected variable: one subscription, values pushed by the
  server, drawn as a sparkline
- `a`/`t`/`f` jump straight to the attribute, type and reference tabs, `y` copies
  the node id, `r` refreshes, `R` re-reads on a timer, `?` lists every key

## Completions

Shell completion comes from [carapace](https://carapace.sh)

```powershell
opcuacli _carapace powershell | Out-String | Invoke-Expression   # add to $PROFILE
```

```sh
opcuacli _carapace bash >> ~/.bashrc        # or zsh, fish, nushell, elvish, xonsh
```

The values are real: profiles from the configuration file, attribute and
reference type names from the same tables the parsers use, and node ids browsed
from the server the active profile points at.
