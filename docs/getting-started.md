# Getting Started

This guide will help you get started with Spec2Cloud.

## Prerequisites

Before you begin, ensure you have:

- Python 3.10 or higher
- Node.js 18 or higher
- Azure subscription (for deployment)
- Git

## Installation

### 1. Clone the Repository

```bash
git clone https://github.com/EmeaAppGbb/spec2cloud.git
cd spec2cloud
```

### 2. Install Dependencies

#### Backend Dependencies

```bash
# Install .NET dependencies
dotnet restore
```

#### Frontend Dependencies

```bash
# Install Node.js dependencies
cd frontend
npm install
```

### 3. Configure Environment

Create a `.env` file in the root directory:

```bash
cp .env.example .env
```

Edit `.env` with your configuration:

```env
AZURE_SUBSCRIPTION_ID=your-subscription-id
AZURE_TENANT_ID=your-tenant-id
# Add other required environment variables
```

## Running Locally

### Backend

```bash
dotnet run --project src/Backend/Backend.csproj
```

### Frontend

```bash
cd frontend
npm run dev
```

### Documentation Server

To view this documentation locally:

```bash
# Install documentation dependencies
pip install -r requirements-docs.txt

# Serve the documentation
mkdocs serve
```

The documentation will be available at `http://localhost:8000`

## Next Steps

- Learn about the [Benefits](benefits.md) of Spec2Cloud
- Understand the [Architecture](architecture.md)
- Start [Contributing](contributing.md)
