# Security policy

uptimyctl stores an Uptimy API key on your machine and can change everything in a workspace, so we take reports seriously and respond quickly.

## Reporting a vulnerability

Please **don't open a public issue**. Report it privately through GitHub: on the [Security tab](https://github.com/uptimy/uptimyctl/security), choose **Report a vulnerability**. Include what you found, how to reproduce it, and the version (`uptimyctl version`).

You'll hear back within 3 working days. We'll keep you updated while we fix it, credit you in the release notes if you'd like, and ask you to hold off on public disclosure until a fixed release is out.

## Supported versions

Security fixes go into the latest release. We don't backport to older versions, so please keep uptimyctl up to date: `uptimyctl update`, or the binaries on the [releases page](https://github.com/uptimy/uptimyctl/releases).

## What's in scope

Anything that exposes the key or lets uptimyctl do something you didn't ask for, for example:

- the API key ending up in output, error messages, logs or files other than `~/.config/uptimyctl/config.yaml` (which is written readable by you only)
- weaknesses in `auth login`: the local callback, the PKCE exchange or the code you paste over SSH
- `uptimyctl update` or the install script installing a binary that isn't the release's, checked against its `checksums.txt`
- `apply` changing or removing something its plan didn't show

Weaknesses in Uptimy's API itself are in scope too: report them here or to support@upti.my, and we'll take it from there.
