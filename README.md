# Xenomorph

Xenomorph is a open-source RAT (Remote Administration Tool) that is written using Golang (Server) and Python (Client). Named after the fictional extraterrestrial species from the Alien film series, Xenomorph is designed to be stealthy and powerful.

## Features

- Privilege Escalation (UAC Bypass)
- Basic Stealer (Passwords, Cookies, etc.)
- Screen Capture (Screenshot)

## Prerequisites
- Go 1.23 or higher
- Python 3.12 or higher

## Installation

> [!IMPORTANT]
> Xenomorph is currently in development and only supports downloading the source code from GitHub. We are working on adding support for package managers and other installation methods.

To install Xenomorph, you can download the source code from GitHub and run the following command:

```bash
git clone https://github.com/codevault-llc/xenomorph.git

cd xenomorph

# You have currently two locations you need to run for testing sakes. The client and the server. The server should be hosted so its publically accessible for the client to access. We are using the method: Socket (TCP) for communication between the client and the server.

# Server
go mod download

go build -o xenomorph-server

./xenomorph-server

# Client

# You need to install the required dependencies for the client. You can do this by running the following command:
pip install -r requirements.txt

# Now you can choose between building the client or running the client. For building the client you can run our build script:
**Windows**:
build.bat

# For running the client you can run the following command:
python main.pyw
```

## Disclaimer

This tool is intended for educational purposes only and the author is not responsible for any misuse of this tool. We do not promote hacking or any malicious activities. Use this tool at your own risk.

## License

> [!NOTE]
> Xenomorph's License may change soon. Please check the [LICENSE](LICENSE) file for the most up-to-date information.

Xenomorph is licensed under the GPL-3.0 License. For more information, see the [LICENSE](LICENSE) file.
