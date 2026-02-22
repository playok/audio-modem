# Audio Modem Protocol Specification v1.0

## Overview
PC와 Android 간 3.5mm AUX 케이블을 통한 양방향 파일 전송 프로토콜.

## Physical Layer

### OFDM Parameters
| Parameter | Value |
|-----------|-------|
| Sample Rate | 44100 Hz |
| FFT Size | 512 |
| Cyclic Prefix | 64 samples |
| Symbol Length | 576 samples (~13.06 ms) |
| Frequency Band | 1000 - 20000 Hz |
| Subcarrier Start | Index 12 (~1032 Hz) |
| Subcarrier End | Index 232 (~19992 Hz) |
| Data Subcarriers | ~205 |
| Pilot Subcarriers | 16 |
| Pilot Indices | 15,29,43,57,71,85,99,113,127,141,155,169,183,197,211,225 |

### Modulation Schemes
| Scheme | Bits/Symbol | Data Rate |
|--------|-------------|-----------|
| QPSK | 2 | ~2.5 KB/s |
| 16-QAM | 4 | ~5.1 KB/s |
| 64-QAM | 6 | ~7.7 KB/s |

### Synchronization
1. **Schmidl-Cox Preamble** (2 OFDM symbols)
   - Symbol 1: Even subcarriers only (BPSK, seed=42) → time-domain repetition
   - Symbol 2: All subcarriers (BPSK, seed=43) → fine frequency estimation
2. **Channel Estimation** (1 OFDM symbol)
   - All subcarriers carry known BPSK values (seed=44)

## Data Link Layer

### Frame Format
```
[Type 1B][SeqNum 1B][PayloadLen 2B][Payload 0-1024B][CRC-32 4B]
```

### Frame Types
| Type | Value | Description |
|------|-------|-------------|
| DATA | 0x01 | Data payload |
| ACK | 0x02 | Acknowledgement |
| NACK | 0x03 | Negative acknowledgement |
| CONTROL | 0x04 | Control/negotiation |
| FILE_META | 0x05 | File metadata |
| FILE_END | 0x06 | End of file marker |
| PING | 0x07 | Connection test |
| PONG | 0x08 | Connection response |

### Error Correction
- **Reed-Solomon RS(255,223)**: 32 parity bytes, corrects up to 16 byte errors
- **CRC-32**: IEEE polynomial, 4-byte checksum

### ARQ Protocol
- **Stop-and-Wait**: Send frame → wait ACK → next frame
- ACK Timeout: 500ms
- Max Retries: 3
- Half-duplex turnaround: 50ms

## Application Layer

### File Transfer Sequence
```
Sender                          Receiver
  |--- PING ------>|
  |<------ PONG ---|
  |--- FILE_META ->|  (filename, size, MD5)
  |<------ ACK ----|
  |--- DATA(0) --->|  (≤1024 bytes)
  |<------ ACK ----|
  |--- DATA(1) --->|
  |<------ ACK ----|
  |      ...       |
  |--- FILE_END -->|
  |<------ ACK ----|
```

### FILE_META Payload
```
[FilenameLen 2B][Filename xB][FileSize 8B][MD5 32B]
```
