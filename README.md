# gosh! Go Share ![CI](https://github.com/oxzi/gosh/workflows/CI/badge.svg)

gosh is a simple HTTP file sharing server on which users can upload their files without login or authentication.
All files have a maximum lifetime and are then purged.


## Features

- __Configurability__
  - One short YAML file for everything
  - Custom maximum file size and lifetime
  - MIME type filter and rewriting
  - Templating the index page, also with additional static files
- __Uploading__
  - Configure a shorter file lifetime for each upload
  - Mark files as burn-after-reading to be deleted after first retrieval
  - Uploader receives deletion URL to remove files before their expiration
  - User manual available from the `/` page
  - Web panel to click those settings
  - HTTP POSTing through `curl` or the like
- __Web server modes__
  - Standalone HTTP web server mode
  - FastCGI web server mode
  - Client side caching by HTTP headers `Last-Modified` / `If-Modified-Since` and HTTP status code 304
  - URL prefix support to host, e.g., under `http://example.org/gosh/`
- __Store__
  - Local file and metadata store
  - Uploader's IP address will be stored for legal reasons, anonymous download
  - All data will be purged when file is deleted
- __Hardening__
  - `chroot`ed, privilege dropped, `fork`+`exec`ed daemon
  - `seccomp-bpf` filtered on Linux
  - `pledge` promised on OpenBSD


## Installation
### Generic Installation

Go is required in a recent version; currently 1.19 or later.

```sh
git clone https://github.com/oxzi/gosh.git
cd gosh

go build
```


### NixOS

#### Server module

> __NOTE: THIS SECTION IS CURRENTLY OUTDATED__

On a NixOS system one can configure gosh as a module.
Have look at the example in `contrib/nixos/`.

```nix
# Example configuration to proxy gosh with nginx with a valid HTTPS certificate.

{ config, pkgs, ... }:
{
  imports = [ /path/to/contrib/nixos/ ];  # TODO: copy or link the contrib/nixos/default.nix

  services = {
    gosh = {
      enable = true;
      contactMail = "abuse@example.com";
      listenAddress = "127.0.0.1:30100";

      maxFilesize = "64MiB";
      maxLifetime = "1w";

      mimeMap = [
        { from = "text/html"; to = "text/plain"; }
      ];
    };

    nginx = {
      enable = true;

      recommendedGzipSettings = true;
      recommendedOptimisation = true;
      recommendedTlsSettings = true;
      recommendedProxySettings = true;

      virtualHosts."gosh.example.com" = {
        enableACME = true;
        forceSSL = true;

        locations."/".proxyPass = "http://${config.services.gosh.listenAddress}/";
      };
    };
  };
}
```

#### Program module

On a NixOS system one can also configure `goshy` as a program.
Have look at the example in `contrib/nixos/goshy.nix`.

```nix
# Example configuration to proxy gosh with nginx with a valid HTTPS certificate.

{ config, pkgs, ... }:
{
  imports = [ /path/to/contrib/nixos/goshy.nix ];  # TODO: copy or link the contrib/nixos/goshy.nix

  programs.goshy = {
    enable = true;
    instance = "https://gosh.example.com";
    defaults = {
      burnAfterReading = true;
      printOnlyUrl = false;
      expiryPeriod = "161s";
    };
  };
}
```


### OpenBSD

Start by compiling `gosh` for OpenBSD as described in the generic instructions above.
Then, prepare your system by creating an user, directories and a configuration.

```sh
go build
doas cp gosh /usr/local/sbin/gosh

doas groupadd _gosh
doas useradd -g _gosh -s /sbin/nologin -d /var/empty _gosh

doas mkdir -p /etc/gosh/store
doas cp gosh.yml /etc/gosh/
doas chown -R _gosh:_gosh /etc/gosh/
doas chmod 0700 /etc/gosh/store/

doas -u _gosh vi /etc/gosh/gosh.yml
# store.path to "/etc/gosh/store"
# webserver.listen.protocol to "unix"
# webserver.listen.bound to "/var/www/run/gosh.sock"
# webserver.protocol to "fcgi"
# webserver.item_config to whatever you find reasonable
# webserver.contact to some real email address

doas cp contrib/openbsd/gosh /etc/rc.d/gosh
doas rcctl start gosh
doas rcctl enable gosh
```

Finally, alter your `/etc/httpd.conf` to contain a `server` block like the following one:
```
server "example.org" {
  listen on * tls port 443
  tls {
    certificate "/etc/ssl/example.org.crt"
    key "/etc/ssl/private/example.org.key"
  }

  connection max request body 67108864  # 64M

  location "/.well-known/acme-challenge/*" {
    root "/acme"
    request strip 2
  }
  location "/*" {
    fastcgi socket "/run/gosh.sock"
  }
}
```

Don't forget to `rcctl reload httpd` your configuration changes.

### Docker

Clone repo, alter gosh.yaml according to your needs and build container with contrib/docker/dockerfile

or 

use provided image from dockerhub 

``` docker-compose
version: "3.8"
services:
  gosh:
    image: gorja/gosh:0.6.0
    restart: unless-stopped
    ports:
      - 8080:80
```

or if you prefer docker run command

``` docker run
docker run -p 8080:80 gorja/gosh:0.6.0 
```


## Running gosh

```
Usage of ./gosh:
  -config string
        YAML configuration file
  -verbose
        Verbose logging
```

Please take a look at the provided example configuration in `gosh.yml`.
Create a copy, modify it and run gosh with it.

```
sudo ./gosh -config gosh.yml -verbose
```


## Posting

Files can be submitted via HTTP POST with common tools, e.g., with `curl`.

```sh
# Upload foo.png
curl -F 'file=@foo.png' http://our-server.example/

# Burn after reading:
curl -F 'file=@foo.png' -F 'burn=1' http://our-server.example/

# Set a custom expiry date, e.g., one day:
curl -F 'file=@foo.png' -F 'time=1d' http://our-server.example/

# Or all together:
curl -F 'file=@foo.png' -F 'time=1d' -F 'burn=1' http://our-server.example/

# Print only URL as response:
curl -F 'file=@foo.png' http://our-server.example/?onlyURL
```

For use with the [Weechat-Android relay client](https://github.com/ubergeek42/weechat-android), simply add the `?onlyURL` GET parameter to the URL and enter in the settings under file sharing with no further changes.


## Shell functions and scripts

A `fish` function and a `bash` script allow handily uploading a file to the server of your choice which need to be manually set.
An own installation and deployment of `goshd` is not necessary to use tools.


### Bash, `contrib/bash/`

The bash script is feature complete compared to the possibilities provided by using `curl` or the web interface.
To be able to use the script, add `goshy` to your PATH, make it executable and set the `GOSH_INSTANCE` environment variable.
For learning the usage run `goshy -h`.


### Fish, `contrib/fish/`

The fish function only provides the capability to upload a file and a flag for burn after reading.
To be able to use the function, copy it's content to `~/.config/fish/config.fish`.


## Related Work

Of course, there are already similar projects, for example:

- [0x0](https://git.0x0.st/mia/0x0)
- [Pomf](https://github.com/pomf/pomf)

There is also [darn](https://github.com/CryptoCopter/darn), a gosh fork which enables server-side file encryption.
Back in time, this code was merged into gosh.
However, for the sake of simplicity and because I don't like to trust a remote server, this has been removed again.
