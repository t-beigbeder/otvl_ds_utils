# Cabri user documentation

[documentation index](../../README.md)

## Presentation

Cabri is a HTTP server to expose S3 objects or FileSystem files as resources over HTTP.

Basically:

- GET /root/d1/: list resources under "/d1/"
- HEAD /root/d1/: status 200 or 404
- PUT /root/d2/: mkdir /d2 or S3 equivalent
- PUT /root/d3/d3a/?recursive: mkdir -p /d3/d3a or S3 equivalent
- DELETE /root/d2/: rmdir /d2 or S3 equivalent
- GET /root/d1/f1.txt: get file or S3 object content
- HEAD /root/d1/f1.txt: status 200 or 404 with Checksum (sha256) and Last-modified
- PUT /root/d2/f2.png: put body in file or S3 object
- DELETE /root/d2/f2.png: rm /d2/f2.png or S3 equivalent

## Using the server

### Security notice

The cabri server exposes the data you want as HTTP resources over http.
A reverse proxy such as Apache, nginx or Traefik should be used
to enable https and manage authentication and authorization.

### Command line

The flags to use are provided here:

    $ cabri-server -h
    Usage of cabri-server:
      -addr string
          The host:port to bind the http server
      -config string
          The configuration name: S3Read or FSWrite
      -debug
          Displays debug messages and run gin in debug mode
      -root-dir string
          Root directory if filesystem
      -root-url string
          Root for the URL

### A server providing S3 objects as resources

Export AWS environment variables:

    AWS_ACCESS_KEY_ID=your_AWS_ACCESS_KEY_ID
    AWS_REGION=your_hosting_AWS_REGION
    AWS_SECRET_ACCESS_KEY=your_AWS_SECRET_ACCESS_KEY

Run the server:

    $ cabri-server -addr cabri_server:8080 -config S3Read -root-url s3cabri

Check access:

    $ curl -I http://cabri_server:8080/s3cabri/a_bucket/an_object_path

### A server exposing Filesystem files as resources

Run the server:

    $ cabri-server  -addr cabri_server:8181 -config FSWrite -root-url fscabri -root-dir /home/guest/fscabri

Check access:

    $ curl -I http://cabri_server:8181/fscabri/path-to-directory-under-root-dir/a_file
    $ curl -X PUT -H "Last-Modified: Fri, 26 Apr 2019 17:40:23 GMT" \
      -T /path/to/local/file \
      http://cabri_server:8181/fscabri/path-to-directory-under-root-dir/

### An example client synchronizing S3 to a filesystem

To be used only in development. For production use, you should enable security.

Command line help:

    $ cabri-synchro-client -h
    Usage of cabri-synchro-client:
      -debug
          Displays debug messages and run gin in debug mode
      -source-url string
          Source URL
      -target-url string
          Target URL

Just run

    $ cabri-synchro-client \
      -source-url http://cabri_server:8080/s3cabri/a_bucket \
      -target-url http://other_cabri_server:8181/fscabri/a_bucket

