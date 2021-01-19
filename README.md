# ACI Rogue EP Control monitoring tool
This tool monitors an ACI fabric for Rogue endpoints and clears impacted nodes. Note that this tool **is not** intended for long-term use. It provides a mechanism to assist in safely implementing Rogue EP Control; however, it has shortcommings over the built-in behavior. Specifically, this tool clears Rogue endpoints across the entire node instead of letting an individual Rogue EP time out. This tool should be treated like training wheels for Rogue EP control, i.e. the recommended long-term strategy is to run Rogue EP Control with the built in settings.

No support is implied with this tool. It has been tested against a variety of ACI versions and imposes no known risk, but please take the time to read this document and understand how this tool works.

## Background
As of ACI firmware 3.2, Rogue EP control is a best practice recommendation. The rogue EP feature detects rapid movement of endpoints, and when detected, marks an endpoint as "rogue." A rogue endpoint is pinned to the last known location and held there for the duration of a specified hold timer. The minimum setting for the hold timer is 300s.

Rogue EP control protects against a variety of issues, including malfunctioning NICs, misconfigured hosts, address conflicts, etc. The risk with enabling it is that some environments have ongoing flaps, e.g. misconfigured hosts, that are *not* causing any observable problem. Introducing Rogue EP Control into an environment like this can impose additional risk in that it might pin hosts that were otherwise working and *cause* problems.

This monitors for rogue endpoints and clears them after a specified interval. Doing this allows turning on Rogue EP Control with "training wheels," making it so the feature can be enabled with reduced risk.

## How it works
1. The tool opens a websocket to the APIC and monitors for Rogue EP faults (`F3013` and `F3014` with a lifecycle of `raised`)
2. When Rogue EP faults are discovered, the faults are added to a queue and their age is monitored until a configured delay has expired
3. Once the age has expired, Rogue EPs are cleared on the affected nodes. Note that clearing Rogue EPs is at the **node level**, so may clear other Rogue EPs than the one specified by the fault.

## Usage
Versioned binary release packages are bundled under releases section of this repository. Additionally, the repository can be cloned and the tool can be built by running `go build` in the cloned folder.

All CLI arguments are optional. Required arguments will be prompted for. Run the tool with `--help` to see available options.

## Contributing
Contributions are welcome. Please open an issue or create a pull request.

## License
This tool is licensed under the APACHE 2.0 license and is free to use, modify, and distribute.
