package protocol

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// FileMetadata holds file transfer metadata.
type FileMetadata struct {
	Filename string
	Size     int64
	MD5Hash  string
}

// EncodeFileMeta encodes file metadata into a frame payload.
// Format: [FilenameLen(2B)][Filename][Size(8B)][MD5(32B)]
func EncodeFileMeta(meta *FileMetadata) []byte {
	nameBytes := []byte(meta.Filename)
	md5Bytes := []byte(meta.MD5Hash)

	buf := make([]byte, 2+len(nameBytes)+8+32)
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(nameBytes)))
	copy(buf[2:], nameBytes)
	offset := 2 + len(nameBytes)
	binary.BigEndian.PutUint64(buf[offset:offset+8], uint64(meta.Size))
	copy(buf[offset+8:], md5Bytes)

	return buf
}

// DecodeFileMeta decodes file metadata from a frame payload.
func DecodeFileMeta(data []byte) (*FileMetadata, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("metadata too short")
	}

	nameLen := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+nameLen+8+32 {
		return nil, fmt.Errorf("metadata truncated")
	}

	filename := string(data[2 : 2+nameLen])
	offset := 2 + nameLen
	size := int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	md5Hash := string(data[offset+8 : offset+8+32])

	return &FileMetadata{
		Filename: filename,
		Size:     size,
		MD5Hash:  md5Hash,
	}, nil
}

// ProgressCallback is called with transfer progress updates.
type ProgressCallback func(bytesSent int64, totalBytes int64, status string)

// FileSender handles sending a file over the audio modem.
type FileSender struct {
	transport  *Transport
	chunkSize  int
	onProgress ProgressCallback
}

// NewFileSender creates a new file sender.
func NewFileSender(transport *Transport) *FileSender {
	return &FileSender{
		transport: transport,
		chunkSize: MaxPayloadSize,
	}
}

// SetProgressCallback sets the progress notification callback.
func (fs *FileSender) SetProgressCallback(cb ProgressCallback) {
	fs.onProgress = cb
}

// SendFile sends a file through the audio modem.
func (fs *FileSender) SendFile(filePath string) error {
	// Open file
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	// Get file info
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	// Compute MD5
	hash := md5.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("compute MD5: %w", err)
	}
	md5Hash := hex.EncodeToString(hash.Sum(nil))

	// Reset file position
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek: %w", err)
	}

	// Send FILE_META
	meta := &FileMetadata{
		Filename: filepath.Base(filePath),
		Size:     info.Size(),
		MD5Hash:  md5Hash,
	}

	metaFrame := &Frame{
		Type:       TypeFileMeta,
		PayloadLen: uint16(len(EncodeFileMeta(meta))),
		Payload:    EncodeFileMeta(meta),
	}
	if err := fs.transport.SendFrame(metaFrame); err != nil {
		return fmt.Errorf("send file meta: %w", err)
	}

	fs.progress(0, info.Size(), "Sending file metadata...")

	// Send data chunks
	buf := make([]byte, fs.chunkSize)
	var bytesSent int64

	for {
		n, err := f.Read(buf)
		if n > 0 {
			dataFrame := NewDataFrame(0, buf[:n])
			if err := fs.transport.SendFrame(dataFrame); err != nil {
				return fmt.Errorf("send data chunk: %w", err)
			}
			bytesSent += int64(n)
			fs.progress(bytesSent, info.Size(), fmt.Sprintf("Sending... %d/%d bytes", bytesSent, info.Size()))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	}

	// Send FILE_END
	endFrame := &Frame{
		Type:       TypeFileEnd,
		PayloadLen: 0,
	}
	if err := fs.transport.SendFrame(endFrame); err != nil {
		return fmt.Errorf("send file end: %w", err)
	}

	fs.progress(info.Size(), info.Size(), "Transfer complete")
	log.Printf("File sent: %s (%d bytes, MD5: %s)", meta.Filename, meta.Size, meta.MD5Hash)

	return nil
}

func (fs *FileSender) progress(sent, total int64, status string) {
	if fs.onProgress != nil {
		fs.onProgress(sent, total, status)
	}
}

// FileReceiver handles receiving a file over the audio modem.
type FileReceiver struct {
	transport  *Transport
	outputDir  string
	onProgress ProgressCallback
}

// NewFileReceiver creates a new file receiver.
func NewFileReceiver(transport *Transport, outputDir string) *FileReceiver {
	return &FileReceiver{
		transport: transport,
		outputDir: outputDir,
	}
}

// SetProgressCallback sets the progress notification callback.
func (fr *FileReceiver) SetProgressCallback(cb ProgressCallback) {
	fr.onProgress = cb
}

// ReceiveFile waits for and receives a file.
func (fr *FileReceiver) ReceiveFile(timeout time.Duration) (*FileMetadata, error) {
	// Wait for FILE_META
	metaFrame, err := fr.transport.ReceiveFrame(timeout)
	if err != nil {
		return nil, fmt.Errorf("receive file meta: %w", err)
	}
	if metaFrame.Type != TypeFileMeta {
		return nil, fmt.Errorf("expected FILE_META, got %s", metaFrame.TypeName())
	}

	meta, err := DecodeFileMeta(metaFrame.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode file meta: %w", err)
	}

	log.Printf("Receiving file: %s (%d bytes, MD5: %s)", meta.Filename, meta.Size, meta.MD5Hash)
	fr.progress(0, meta.Size, fmt.Sprintf("Receiving: %s", meta.Filename))

	// Create output file
	outPath := filepath.Join(fr.outputDir, meta.Filename)
	outFile, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	defer outFile.Close()

	// Receive data chunks
	hash := md5.New()
	var bytesReceived int64

	for bytesReceived < meta.Size {
		frame, err := fr.transport.ReceiveFrame(5 * time.Second)
		if err != nil {
			return nil, fmt.Errorf("receive data chunk: %w", err)
		}

		switch frame.Type {
		case TypeData:
			n, err := outFile.Write(frame.Payload[:frame.PayloadLen])
			if err != nil {
				return nil, fmt.Errorf("write data: %w", err)
			}
			hash.Write(frame.Payload[:frame.PayloadLen])
			bytesReceived += int64(n)
			fr.progress(bytesReceived, meta.Size,
				fmt.Sprintf("Receiving... %d/%d bytes", bytesReceived, meta.Size))

		case TypeFileEnd:
			goto done

		default:
			log.Printf("Unexpected frame type during transfer: %s", frame.TypeName())
		}
	}

done:
	// Wait for FILE_END if we haven't received it yet
	if bytesReceived >= meta.Size {
		endFrame, err := fr.transport.ReceiveFrame(2 * time.Second)
		if err == nil && endFrame.Type != TypeFileEnd {
			log.Printf("Expected FILE_END, got %s", endFrame.TypeName())
		}
	}

	// Verify MD5
	receivedMD5 := hex.EncodeToString(hash.Sum(nil))
	if receivedMD5 != meta.MD5Hash {
		return nil, fmt.Errorf("MD5 mismatch: expected %s, got %s", meta.MD5Hash, receivedMD5)
	}

	fr.progress(meta.Size, meta.Size, "Transfer complete - MD5 verified")
	log.Printf("File received: %s (%d bytes, MD5 verified)", meta.Filename, meta.Size)

	return meta, nil
}

func (fr *FileReceiver) progress(received, total int64, status string) {
	if fr.onProgress != nil {
		fr.onProgress(received, total, status)
	}
}
