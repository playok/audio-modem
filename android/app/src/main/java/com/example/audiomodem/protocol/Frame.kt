package com.example.audiomodem.protocol

import com.example.audiomodem.fec.CRC32

/**
 * Protocol frame encoding/decoding.
 * Format: [Type(1B)][SeqNum(1B)][PayloadLen(2B)][Payload][CRC-32(4B)]
 */
object FrameType {
    const val DATA: Byte = 0x01
    const val ACK: Byte = 0x02
    const val NACK: Byte = 0x03
    const val CONTROL: Byte = 0x04
    const val FILE_META: Byte = 0x05
    const val FILE_END: Byte = 0x06
    const val PING: Byte = 0x07
    const val PONG: Byte = 0x08
}

const val HEADER_SIZE = 4
const val MAX_PAYLOAD_SIZE = 1024
const val CRC_SIZE = 4

data class Frame(
    val type: Byte,
    val seqNum: Byte,
    val payloadLen: Int,
    val payload: ByteArray?
) {
    fun typeName(): String = when (type) {
        FrameType.DATA -> "DATA"
        FrameType.ACK -> "ACK"
        FrameType.NACK -> "NACK"
        FrameType.CONTROL -> "CONTROL"
        FrameType.FILE_META -> "FILE_META"
        FrameType.FILE_END -> "FILE_END"
        FrameType.PING -> "PING"
        FrameType.PONG -> "PONG"
        else -> "UNKNOWN(0x${type.toString(16)})"
    }

    fun encode(): ByteArray {
        val totalLen = HEADER_SIZE + payloadLen + CRC_SIZE
        val buf = ByteArray(totalLen)

        buf[0] = type
        buf[1] = seqNum
        buf[2] = (payloadLen shr 8 and 0xFF).toByte()
        buf[3] = (payloadLen and 0xFF).toByte()

        if (payloadLen > 0 && payload != null) {
            payload.copyInto(buf, HEADER_SIZE, 0, payloadLen)
        }

        // CRC over header + payload
        val dataForCRC = buf.copyOf(HEADER_SIZE + payloadLen)
        val crc = CRC32.compute(dataForCRC)
        buf[totalLen - 4] = (crc shr 24 and 0xFF).toByte()
        buf[totalLen - 3] = (crc shr 16 and 0xFF).toByte()
        buf[totalLen - 2] = (crc shr 8 and 0xFF).toByte()
        buf[totalLen - 1] = (crc and 0xFF).toByte()

        return buf
    }

    companion object {
        fun decode(data: ByteArray): Frame {
            require(data.size >= HEADER_SIZE + CRC_SIZE) { "Frame too short: ${data.size}" }

            val type = data[0]
            val seqNum = data[1]
            val payloadLen = ((data[2].toInt() and 0xFF) shl 8) or (data[3].toInt() and 0xFF)

            val expectedLen = HEADER_SIZE + payloadLen + CRC_SIZE
            require(data.size >= expectedLen) { "Frame truncated: ${data.size} < $expectedLen" }

            // Verify CRC
            val dataForCRC = data.copyOf(HEADER_SIZE + payloadLen)
            val expectedCRC = ((data[expectedLen - 4].toLong() and 0xFF) shl 24) or
                              ((data[expectedLen - 3].toLong() and 0xFF) shl 16) or
                              ((data[expectedLen - 2].toLong() and 0xFF) shl 8) or
                              (data[expectedLen - 1].toLong() and 0xFF)
            val actualCRC = CRC32.compute(dataForCRC)

            require(expectedCRC == actualCRC) {
                "CRC mismatch: expected 0x${expectedCRC.toString(16)}, got 0x${actualCRC.toString(16)}"
            }

            val payload = if (payloadLen > 0) data.copyOfRange(HEADER_SIZE, HEADER_SIZE + payloadLen) else null

            return Frame(type, seqNum, payloadLen, payload)
        }

        fun dataFrame(seqNum: Byte, payload: ByteArray) =
            Frame(FrameType.DATA, seqNum, payload.size, payload)

        fun ackFrame(seqNum: Byte) = Frame(FrameType.ACK, seqNum, 0, null)
        fun nackFrame(seqNum: Byte) = Frame(FrameType.NACK, seqNum, 0, null)
        fun pingFrame() = Frame(FrameType.PING, 0, 0, null)
        fun pongFrame() = Frame(FrameType.PONG, 0, 0, null)

        fun fileMetaFrame(filename: String, size: Long, md5: String): Frame {
            val nameBytes = filename.toByteArray()
            val md5Bytes = md5.toByteArray()
            val payload = ByteArray(2 + nameBytes.size + 8 + 32)

            payload[0] = (nameBytes.size shr 8 and 0xFF).toByte()
            payload[1] = (nameBytes.size and 0xFF).toByte()
            nameBytes.copyInto(payload, 2)
            val off = 2 + nameBytes.size
            for (i in 0 until 8) payload[off + i] = (size shr ((7 - i) * 8) and 0xFF).toByte()
            md5Bytes.copyInto(payload, off + 8)

            return Frame(FrameType.FILE_META, 0, payload.size, payload)
        }

        fun fileEndFrame() = Frame(FrameType.FILE_END, 0, 0, null)
    }

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is Frame) return false
        return type == other.type && seqNum == other.seqNum && payloadLen == other.payloadLen
    }

    override fun hashCode(): Int = type.hashCode() * 31 + seqNum.hashCode()
}
