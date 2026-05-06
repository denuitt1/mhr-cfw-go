# MHR-CFW Go Port
[![GitHub](https://img.shields.io/badge/GitHub-MHR_CFW-blue?logo=github)](https://github.com/denuitt1/mhr-cfw)

| [English](README.md) | [Persian](README_FA.md) |
| :---: | :---: |

---

## How It Works

1 - GAS Direct
```
Client -> Local Relay -> Google/CDN Front -> GAS (Google Apps Script) Relay -> Exit
            |
            +-> Shows www.google.com to network DPI filter
```

2 - GAS + Cloudflare Worker Exit
```
Client -> Local Relay -> Google/CDN Front -> GAS (Google Apps Script) Relay -> Cloudflare Worker -> Exit
            |
            +-> Shows www.google.com to network DPI filter
```

3 - GAS + Cloudflare Worker Middle + Self-Hosted Upstream Forwarder Relay Exit
```
Client -> Local Relay -> Google/CDN Front -> GAS (Google Apps Script) Relay -> Cloudflare Worker -> Self-Hosted Upstream Forwarder -> Exit
            |
            +-> Shows www.google.com to network DPI filter
```

In normal use, the browser sends traffic to the proxy running on your computer.
The proxy sends that traffic through Google-facing infrastructure so the network only sees an allowed domain such as `www.google.com`.
Your deployed relay then fetches the real website through cloudflare worker and sends the response back through the same path.

This means the filter sees normal-looking Google traffic, while the actual destination stays hidden inside the relay request.


## Disclaimer

`mhr-cfw-go` is provided for educational, testing, and research purposes only.

- **Provided without warranty:** This software is provided "AS IS", without express or implied warranty, including merchantability, fitness for a particular purpose, and non-infringement.
- **Limitation of liability:** The developers and contributors are not responsible for any direct, indirect, incidental, consequential, or other damages resulting from the use of this project or the inability to use it.
- **User responsibility:** Running this project outside controlled test environments may affect networks, accounts, proxies, certificates, or connected systems. You are solely responsible for installation, configuration, and use.
- **Legal compliance:** You are responsible for complying with all local, national, and international laws and regulations before using this software.
- **Google services compliance:** If you use Google Apps Script or other Google services with this project, you are responsible for complying with Google's Terms of Service, acceptable use rules, quotas, and platform policies. Misuse may lead to suspension or termination of your Google account or deployments.
- **License terms:** Use, copying, distribution, and modification of this software are governed by the repository license. Any use outside those terms is prohibited.

---

## Original Projects
#### Based on [ThisIsDara/mhr-cfw-go](https://github.com/ThisIsDara/mhr-cfw-go), the Go implementation of [mhr-cfw](https://github.com/denuitt1/mhr-cfw).


## License

MIT
