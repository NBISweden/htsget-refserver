Developing the htsget refserver
===============================

## TLS

Developing while using TLS can be a challenge as you'll likely be working with
self-signed CA certificates. To make the server trust these certificates, there
are some things that can be done.

To start the server trusting a local CA certificate, you can set the
`SSL_CERT_FILE` environment variable, pointing to the certificate.

ex.
```bash
$ SSL_CERT_FILE=ca.pem go run cmd/main.go -config config.json
```

This will have to be done for the `htsget` client as well, which is done
slightly differently.

ex.
```bash
$ REQUESTS_CA_BUNDLE=ca.pem htsget --bearer-token "$token" http://localhost:3000/reads/test --output test.file
```

For `samtools`, there are a few things that needs to be done. First of all, an
the `HTS_ALLOW_UNENCRYPTED_AUTHORIZATION_HEADER` needs to be set to
`I understand the risks`.
Secondly, the token needs to be put in a text file, identified by the
`HTS_AUTH_LOCATION` environment variable. Lastly `CURL_CA_BUNDLE` needs to be
pointed to your local ca certificate (unless you have it locally installed).

ex.
```bash
$ export HTS_ALLOW_UNENCRYPTED_AUTHORIZATION_HEADER="I understand the risks"
$ export HTS_AUTH_LOCATION=token.txt
$ export CURL_CA_BUNDLE=certs/ca.pem
$ echo "$(get_token)" > token.txt
$ samtools view http://localhost:3000/reads/test
```
