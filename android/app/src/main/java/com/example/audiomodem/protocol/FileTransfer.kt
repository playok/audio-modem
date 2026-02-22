package com.example.audiomodem.protocol

import android.content.Context
import android.net.Uri
import android.util.Log
import java.io.File
import java.security.MessageDigest

/**
 * File transfer over the audio modem transport.
 */
class FileTransfer(private val transport: Transport) {

    companion object {
        private const val TAG = "FileTransfer"
        const val CHUNK_SIZE = MAX_PAYLOAD_SIZE
    }

    var onProgress: ((sent: Long, total: Long, message: String) -> Unit)? = null

    /**
     * Send a file from the given URI.
     */
    suspend fun sendFile(context: Context, uri: Uri, filename: String): Boolean {
        val inputStream = context.contentResolver.openInputStream(uri) ?: return false
        val data = inputStream.use { it.readBytes() }

        val md5 = MessageDigest.getInstance("MD5").digest(data)
        val md5Hex = md5.joinToString("") { "%02x".format(it) }

        Log.d(TAG, "Sending: $filename (${data.size} bytes, MD5: $md5Hex)")

        // Send FILE_META
        val metaFrame = Frame.fileMetaFrame(filename, data.size.toLong(), md5Hex)
        if (!transport.sendFrame(metaFrame)) return false

        onProgress?.invoke(0, data.size.toLong(), "Sending metadata...")

        // Send data chunks
        var offset = 0
        while (offset < data.size) {
            val chunkSize = minOf(CHUNK_SIZE, data.size - offset)
            val chunk = data.copyOfRange(offset, offset + chunkSize)
            val dataFrame = Frame.dataFrame(0, chunk)

            if (!transport.sendFrame(dataFrame)) {
                Log.e(TAG, "Failed to send chunk at offset $offset")
                return false
            }

            offset += chunkSize
            onProgress?.invoke(offset.toLong(), data.size.toLong(),
                "Sending... $offset/${data.size} bytes")
        }

        // Send FILE_END
        if (!transport.sendFrame(Frame.fileEndFrame())) return false

        onProgress?.invoke(data.size.toLong(), data.size.toLong(), "Transfer complete!")
        Log.d(TAG, "File sent successfully")
        return true
    }

    /**
     * Receive a file and save to the output directory.
     */
    suspend fun receiveFile(outputDir: File, timeoutMs: Long): FileMetadata? {
        // Wait for FILE_META
        val metaFrame = transport.receiveFrame(timeoutMs) ?: return null
        if (metaFrame.type != FrameType.FILE_META || metaFrame.payload == null) return null

        val meta = decodeFileMeta(metaFrame.payload)
        Log.d(TAG, "Receiving: ${meta.filename} (${meta.size} bytes)")
        onProgress?.invoke(0, meta.size, "Receiving: ${meta.filename}")

        // Receive data chunks
        outputDir.mkdirs()
        val outFile = File(outputDir, meta.filename)
        val buffer = mutableListOf<Byte>()

        while (buffer.size < meta.size) {
            val frame = transport.receiveFrame(5000) ?: return null

            when (frame.type) {
                FrameType.DATA -> {
                    if (frame.payload != null) {
                        buffer.addAll(frame.payload.toList())
                        onProgress?.invoke(buffer.size.toLong(), meta.size,
                            "Receiving... ${buffer.size}/${meta.size} bytes")
                    }
                }
                FrameType.FILE_END -> break
                else -> Log.w(TAG, "Unexpected frame: ${frame.typeName()}")
            }
        }

        outFile.writeBytes(buffer.toByteArray())

        // Verify MD5
        val receivedMd5 = MessageDigest.getInstance("MD5")
            .digest(buffer.toByteArray())
            .joinToString("") { "%02x".format(it) }

        if (receivedMd5 != meta.md5) {
            Log.e(TAG, "MD5 mismatch: expected ${meta.md5}, got $receivedMd5")
            return null
        }

        onProgress?.invoke(meta.size, meta.size, "Transfer complete - MD5 verified!")
        Log.d(TAG, "File received: ${meta.filename}")
        return meta
    }

    data class FileMetadata(
        val filename: String,
        val size: Long,
        val md5: String
    )

    private fun decodeFileMeta(data: ByteArray): FileMetadata {
        val nameLen = ((data[0].toInt() and 0xFF) shl 8) or (data[1].toInt() and 0xFF)
        val filename = String(data, 2, nameLen)
        val off = 2 + nameLen
        var size = 0L
        for (i in 0 until 8) {
            size = (size shl 8) or (data[off + i].toLong() and 0xFF)
        }
        val md5 = String(data, off + 8, 32)
        return FileMetadata(filename, size, md5)
    }
}
