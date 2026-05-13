# Security Policy

## Supported Versions

Security fixes are published for the latest `main` branch unless a release branch is explicitly announced.

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| older   | :x:                |

## Reporting a Vulnerability

Please do not open public issues for suspected vulnerabilities.

Report privately through GitHub Security Advisories:

https://github.com/vancehuds/VanceFiveMLog/security/advisories/new

Include:

- affected version or commit
- deployment mode and environment
- reproduction steps
- impact and any logs or screenshots that help explain the issue

If GitHub Security Advisories are unavailable, contact the repository owner privately before publishing details.

## Security Best Practices

### Production Deployments

- Set `APP_ENV=production` to enable secure session cookies
- Use a unique `SESSION_SECRET` of at least 32 characters
- Enable TLS/HTTPS in production
- Configure firewall rules appropriately

### Admin Security

- Use strong passwords for admin accounts
- Regularly rotate `SESSION_SECRET`
- Enable Cloudflare Turnstile for additional login protection
- Monitor the log review workflow for suspicious activity

### API Keys

- Server API keys are stored as SHA-256 hashes
- Keep API keys confidential and rotate if compromised
- Use environment variables or secure configuration for API keys

## Dependencies

This project is built with Go and uses the following security-related dependencies:

- `golang.org/x/crypto` - for secure password hashing and session management

Report any vulnerabilities in dependencies through GitHub Security Advisories.
