This repository contains the source code of `cct`, a command-line application for getting and setting CC and sysex-encoded patch data from various vintage synthesizers.



Supported synths are:
- Nord Lead/Rack 2X
- Nord Drum 2
- Elektron Machinedrum SPS-1 UW Mk2

## Usage

To compile (requires Go v1.22):
```
make build
```

To compile and install to `/usr/local/bin`:
```
make install
```

For full usage:
```
cct -h
```

Note that individual commands have their own usage instructions, for example `cct nd2 -h` shows the usage of the Nord Drum 2 sub-command.

### Example

To save the patch in slot B of a Nord Lead 2X in CSV format into the file `patch.csv``:
```
cct nr2x get -c B ./patch.csv
```