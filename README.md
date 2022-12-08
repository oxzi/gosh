# gosh! Go Share ![CI](https://github.com/oxzi/gosh/workflows/CI/badge.svg)

gosh is a simple HTTP file sharing server on which users can upload their files without login or authentication.
All files have a maximum lifetime and are then deleted.


## Features

- Standalone HTTP web server, no additional server needed (might be proxied for HTTPS)
- Supports a FastCGI server instead of HTTP if `--listen fcgi:/path/to.sock`
- Store with both files and some metadata
- Only safe uploader's IP address for legal reasons, anonymous download
- File and all metadata are automatically deleted after expiration
- Configurable maximum lifetime and file size for uploads
- Replace or drop configured MIME types
- Simple upload both via webpanel or by `curl`, `wget` or the like
- User manual available from the `/` page
- Uploads can specify their own shorter lifetime
- Each upload generates also generates a custom deletion URL
- Burn after Reading: uploads can be deleted after the first download
- Client side caching by HTTP headers `Last-Modified` and `If-Modified-Since` and HTTP status code 304
- On Linux there is additional hardening through Landlock and seccomp-bpf
- On OpenBSD there is additional hardening through pledge and unveil
- Print only the final UR with the `?onlyURL` GET parameter instead of a more verbose output


## Installation
### Generic Installation

Go is required in a recent version; currently 1.17 or later.

```bash
git clone https://github.com/oxzi/gosh.git
cd gosh

CGO_ENABLED=0 go build -gcflags="all=-N -l" ./cmd/goshd
CGO_ENABLED=0 go build -gcflags="all=-N -l" ./cmd/gosh-query
```

### NixOS

On a NixOS system one can configure gosh as a module. Have look at the example in `contrib/nixos/`.

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

### OpenBSD

Start by (cross-) compiling `goshd` for OpenBSD as described in the generic instructions above.

Afterwards copy the `./contrib/openbsd/goshd-rcd` rc.d file to `/etc/rc.d/goshd` and modify if necessary.
You should at least replace `example.org` by your domain and create the directories below `/var/www`.

The service can be activated through:
```
rcctl set goshd flags -max-filesize 64MiB -max-lifetime 3d -contact gosh-abuse@example.org
rcctl enable goshd
rcctl start goshd
```

Your `/etc/httpd.conf` should contain a `server` block like the following one:
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

## Commands
### goshd

`goshd` is the web server, as described above.

```
Usage of ./goshd:
  -contact string
        Contact E-Mail for abuses
  -listen string
        Listen address for the HTTP server (default ":8080")
  -max-filesize string
        Maximum file size in bytes (default "10MiB")
  -max-lifetime string
        Maximum lifetime (default "24h")
  -mimemap string
        MimeMap to substitute/drop MIMEs
  -store string
        Path to the store
  -verbose
        Verbose logging
```

An example usage could look like this.

```bash
./goshd \
  -contact my@email.address \
  -max-filesize 64MiB \
  -max-lifetime 2w \
  -mimemap Mimemap \
  -store /path/to/my/store/dir
```

The *MimeMap* file contains both substitutions or *drops* in each line and could look as follows.

```
# Replace text/html with text/plain
text/html text/plain

# Drop PNGs, because reasons.
image/png DROP
```


### gosh-query

The store can also be queried offline to get information or delete items. This is `gosh-query`'s job.

```
Usage of ./gosh-query:
  -delete
        Delete selection
  -id string
        Query for an ID
  -ip-addr string
        Query for an IP address
  -store string
        Path to the store
  -verbose
        Verbose logging
```

```bash
# Find all uploads from the localhost:
./gosh-query -store /path/to/store -ip-addr ::1

# Show information for upload with ID abcdef
./gosh-query -store /path/to/store -id abcdef

# And delete this one
./gosh-query -store /path/to/store -delete -id abcdef
```

## Posting

Files can be submitted via HTTP POST with common tools, e.g., with `curl`.

```bash
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

## Shell function

### Fish, `contrib/fish/`

A `fish`-function allow to easily upload a file to the server of your choice which needs to manually edited.
With the `-b` or `--burn` flags you may decide to burn the file after reading.


## Related Work

Of course, there are already similar projects, for example:

- [0x0](https://git.0x0.st/mia/0x0)
- [Pomf](https://github.com/pomf/pomf)

There is also [darn](https://github.com/CryptoCopter/darn), a gosh fork which enables server-side file encryption.
Back in time, this code was merged into gosh.
However, for the sake of simplicity and because I don't like to trust a remote server, this has been removed again.
