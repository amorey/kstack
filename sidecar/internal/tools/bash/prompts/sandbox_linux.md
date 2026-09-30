
A program installed as a snap, one under `/snap/bin`, does not run in the sandbox, since it starts through snapd. Set `dangerouslyDisableSandbox` for a command that runs one, such as a snap's `kubectl` or `helm`.
