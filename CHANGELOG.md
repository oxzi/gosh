# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog][keep-a-changelog], and this project adheres to [Semantic Versioning][semantic-versioning].

<!--
Please keep the text width at 72 chars for easy copying into git tags.


Types of changes:

- Added       for new features.
- Changed     for changes in existing functionality.
- Deprecated  for soon-to-be removed features.
- Removed     for now removed features.
- Fixed       for any bug fixes.
- Security    in case of vulnerabilities
-->

## [Unreleased]
### Added
- URL prefix support to host, e.g., under `http://example.org/gosh/`
- Add goshy as bash script and NixOS program, [@riotbib](https://github.com/riotbib) in [#27](https://github.com/oxzi/gosh/pull/27).
- Created Store RPC working on Unix domain sockets to allow a `fork`+`exec`ed daemon.
- Configuration through YAML configuration file.
- Configurable index template and static files, partially by [@riotbib](https://github.com/riotbib) in [#45](https://github.com/oxzi/gosh/pull/45).

### Changed
- Dependency version bumps.
- Great structural refactoring.
- `goshd` became `gosh`.
- Made `gosh` a `chroot`ed, privilege dropped, `fork`+`exec`ed daemon.
- OpenBSD installation changed due to structural program changes.
- Bumped required Go version from 1.19 to 1.21.
- Replaced logrus logging with Go's new `log/slog` and do wrapping for child processes.

### Deprecated
### Removed
- The `gosh-query` utility was removed.
- `gosh` lost most of its command line arguments due to the YAML configuration.
- Removed path filtering (Linux' Landlock LSM, OpenBSD's `unveil`) as `gosh` is always `chroot`ed now.

### Fixed
- OpenBSD rc.d file for OpenBSD 7.3 or later.
- Forward web requests to main page if URL is above prefixed root.

### Security


## [0.6.0] - 2022-11-19
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

- FastCGI listener support next to a HTTP daemon.
- OpenBSD and generic Unix hardening.
- Reduce badger database's mmaped storage size.
- Configure Renovate for repository.


## [0.5.0] - 2022-08-07
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

- Client side caching with HTTP 304 (Not Modified)
- Landlock path restriction hardening on Linux


## [0.4.1] - 2022-05-03
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

- GET parameter to only print URL, thanks @riotbib #7


## [0.4.0] - 2022-03-26
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

- File deletion through newly generated deletion URLs.
- Database update: Warning, this breaks compatibility! Please prepare
  your old database by using the "rm" migration tool.
- Use seccomp-bpf for both goshd and gosh-query.
- Some small refactoring.


## [0.3.1] - 2022-03-26
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

- goshd: replace static syscall list with syscallset
- Revert "goshd: serve webserver within a gzip wrapper"
  Only lead to more problems and compression was added by nginx anyway.
- contrib/nixos: harden systemd unit


## [0.3.0] - 2021-04-26
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

goshd: seccomp hardening and gzip encoding


## [0.2.0] - 2019-09-02
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

Interactive HTML index page


## [0.1.1] - 2019-08-07
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

Fix small bugs, NixOS Module


## [0.1.0] - 2019-08-07
> _This release was created before adapting the [Keep a Changelog][keep-a-changelog] format._

First release


[keep-a-changelog]: https://keepachangelog.com/en/1.1.0/
[semantic-versioning]: https://semver.org/spec/v2.0.0.html

[0.1.0]: https://github.com/oxzi/gosh/releases/tag/v0.1.0
[0.1.1]: https://github.com/oxzi/gosh/compare/v0.1.0...v0.1.1
[0.2.0]: https://github.com/oxzi/gosh/compare/v0.1.1...v0.2.0
[0.3.0]: https://github.com/oxzi/gosh/compare/v0.2.0...v0.3.0
[0.3.1]: https://github.com/oxzi/gosh/compare/v0.3.0...v0.3.1
[0.4.0]: https://github.com/oxzi/gosh/compare/v0.3.1...v0.4.0
[0.4.1]: https://github.com/oxzi/gosh/compare/v0.4.0...v0.4.1
[0.5.0]: https://github.com/oxzi/gosh/compare/v0.4.1...v0.5.0
[0.6.0]: https://github.com/oxzi/gosh/compare/v0.5.0...v0.6.0
[Unreleased]: https://github.com/oxzi/gosh/compare/v0.6.0...main
