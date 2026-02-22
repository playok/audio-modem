# Audio Modem

A browser-based OFDM audio modem file transfer system.

Transfer files between two devices using speakers and microphones, or a 3.5mm AUX cable. No server required — just open `index.html`.

[한국어](README.md)

## Features

- **No server required** — Pure client-side JavaScript, works with static files only
- **OFDM modulation** — High-speed data transfer using multiple subcarriers
- **Multiple modulation schemes** — QPSK, 16-QAM, BPSK (acoustic/high-reliability/narrowband)
- **Large file support** — Chunked transfer + streaming receiver handles 500MB+ files
- **CRC-32 verification** — Per-frame integrity checking
- **Real-time monitoring** — Level meter, waveform trimmer, chunk bitmap visualization

## Quick Start

```bash
# Option 1: Open directly
open index.html

# Option 2: Local server (HTTPS/localhost required for microphone)
python3 -m http.server 8000
# Visit http://localhost:8000
```

## Usage

### Sending

1. Select **File Send** mode
2. Choose modulation (cable: QPSK/16-QAM, speaker: BPSK)
3. Select a file and click **Start Send**

### Receiving

1. Select **File Receive** mode
2. Choose receive method:
   - **Manual (Trim)** — For small files; record, select range, then demodulate
   - **Streaming (Real-time)** — For large files; automatic frame detection and demodulation
3. Click **Start Receive**, then start sending from the other device

## Modulation Schemes

| Scheme | Speed | Use Case |
|--------|-------|----------|
| QPSK | ~2.5 KB/s | Cable connection (default) |
| 16-QAM | ~5 KB/s | Cable connection (high speed) |
| BPSK | ~0.5 KB/s | Speaker → Microphone |
| BPSK-Repeat | ~170 B/s | Noisy environments, high reliability |
| Narrowband | ~100 B/s | Maximum stability |

## Large File Transfer

Files exceeding 32KB are automatically split into chunks:

- **Send**: File split into 2–4KB chunks, each transmitted as an independent OFDM frame
- **Receive**: Real-time preamble detection → frame demodulation → IndexedDB storage
- **Memory**: Constant O(chunkSize) memory usage on both sides

## Technical Details

- **Modulation**: OFDM (512-point FFT, ~205 data subcarriers)
- **Synchronization**: Schmidl-Cox preamble (auto-correlation + cross-correlation)
- **Channel estimation**: Pilot subcarriers + CE symbol
- **Error detection**: CRC-32
- **Audio**: Web Audio API (44100 Hz)
- **Storage**: IndexedDB (for large file chunk storage)

## File Structure

```
index.html  — UI (send/receive panels, settings, progress)
modem.js    — OFDM core (FFT, modulation, preamble, chunk protocol)
app.js      — App logic (send/receive, streaming, UI control)
docs/       — Protocol specification
```

## Browser Compatibility

HTTPS or localhost is required for microphone access.

- Chrome 66+
- Firefox 76+
- Safari 14.1+
- Edge 79+

## License

MIT
