
# Bitcoin Payment Button

The Bitcoin Payment Button is a simple Go-based backend application that generates a dynamic Bitcoin address and corresponding QR code for accepting payments. It integrates with a chosen Bitcoin payment processing system to facilitate seamless transactions.

## Features

- Generates a unique Bitcoin address for each payment request
- Creates a QR code for easy scanning of the Bitcoin address
- Integrates with a chosen Bitcoin payment processing system
- Provides an API endpoint for retrieving the payment details and QR code URL

## Prerequisites

- Go 1.16 or later
- Dependencies managed using Go modules

## Installation

1. Clone the repository:

```bash
git clone https://github.com/ngenohkevin/paybutton.git
cd paybutton
```

2. Install the required dependencies:

```bash
go mod download
```

## Configuration

1. Create a `.env` file.
2. Name it to `BLOCKONOMICS_API_KEY=`
3. Save the file.

## Usage

1. Start the Go server:

```bash
go run main.go
```

2. The server will be running on `http://localhost:8080` by default.

## API Endpoints

### Generate Payment

- **URL**: `/payment`
- **Method**: `POST`
- **Request Body**:

```json
{
  "email": "user@example.com",
  "price": 100.0
}
```

- **Response**:

```json
{
  "address": "bc1qnmhyc7kqu4tlzfmtkwhtjlg2zfzl95fkgc2p29",
  "qrCodeURL": "/bc1qnmhyc7kqu4tlzfmtkwhtjlg2zfzl95fkgc2p29.png",
  "email": "user@example.com",
  "priceInBTC": 0.0037451640569115133,
  "priceInUSD": 100
}
```

## Dependencies

- [github.com/gin-gonic/gin](https://github.com/gin-gonic/gin): HTTP web framework
- [github.com/skip2/go-qrcode](https://github.com/skip2/go-qrcode): QR code generation library

## Contributing

Contributions to the Bitcoin Payment project are welcome! If you find a bug or have suggestions for improvement, please feel free to open an issue or submit a pull request.

## License

This project is licensed under the [MIT License](LICENSE).

---
