# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub: **Security → Report a vulnerability** on this repository (private vulnerability reporting). Do not open a public issue.

Include the affected version (`aitk version`), steps to reproduce, and the impact you expect. We aim to acknowledge reports within 3 working days and to ship a fix or mitigation for confirmed high-severity issues within 30 days, crediting you unless you prefer otherwise.

## Supported versions

| Version | Supported |
|---|---|
| 2.x | yes |
| 1.x (bash) | security fixes only until 2027-04-01 |

## Verifying releases

Release archives are listed with SHA-256 sums in `checksums.txt`, which is signed with Sigstore cosign (keyless, GitHub Actions identity):

```bash
cosign verify-blob --certificate checksums.txt.pem --signature checksums.txt.sig \
  --certificate-identity-regexp '^https://github.com/innaka-tech/ai-toolkit/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

`install.sh` performs the checksum check always and the signature check when cosign is installed.
