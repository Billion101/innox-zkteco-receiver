# INNO X — ZKTeco SenseFace 4A ADMS Receiver

A standalone high-performance Cloud Ingestion Service for **ZKTeco SenseFace 4A** (and compatible ZKTeco biometric terminals) over the ADMS HTTP Push protocol.

## Features
- **ADMS Protocol Compliance:**
  - `GET /iclock/cdata`: Handshake & device options negotiation.
  - `POST /iclock/cdata`: Punch log ingestion (Face Scan, Fingerprint, Card, Password).
  - `GET /iclock/getrequest`: Heartbeat & device status tracking.
  - `POST /iclock/devicecmd`: Command acknowledgment.
- **Smart Check-In / Check-Out Logic:**
  - First punch of day $\rightarrow$ `CHECK_IN` (`ON_TIME` if $\le$ 09:00, `LATE` if $>$ 09:00).
  - Subsequent punches within 5 minutes $\rightarrow$ `DUPLICATE_IGNORED` (Debounce).
  - Later punches $\rightarrow$ `CHECK_OUT` with calculated work duration.
- **Biometric PIN Mapping:**
  - Maps physical device PINs (9–16) to employees (Billion, Saymisouk, TINAR, etc.).
- **Live Realtime Monitoring Dashboard:**
  - Modern web UI with Server-Sent Events (SSE) stream on port `8087`.
  - Manual test punch simulator.
- **Persistence:**
  - Automated PostgreSQL schema & seed migration.
  - In-memory fallback if database is detached.

## Quick Start
```bash
docker compose up -d --build
```
Access Dashboard at: `http://<SERVER_IP>:8087/`

## Testing with Bruno
A ready-to-use Bruno API collection is provided under the [`bruno/`](file:///Users/mac2hand/Desktop/innox/innox-zkteco-receiver/bruno) folder:
- Open Bruno, choose **Open Collection**, and select the `bruno` directory.
- Select the `Local` environment (`baseUrl`: `http://localhost:8087`, `deviceSN`: `PYA8262400422`).
- Included folders:
  - `01-adms-protocol`: Simulates device communication (`cdata`, `getrequest`, `devicecmd`, `fdata`).
  - `02-monitoring-api`: APIs for dashboard stats, devices, events SSE stream, and JSON test punch.

