package com.example.audiomodem.fec

import java.util.zip.CRC32 as JavaCRC32

object CRC32 {
    fun compute(data: ByteArray): Long {
        val crc = JavaCRC32()
        crc.update(data)
        return crc.value
    }

    fun append(data: ByteArray): ByteArray {
        val checksum = compute(data)
        val result = ByteArray(data.size + 4)
        data.copyInto(result)
        result[data.size] = (checksum shr 24 and 0xFF).toByte()
        result[data.size + 1] = (checksum shr 16 and 0xFF).toByte()
        result[data.size + 2] = (checksum shr 8 and 0xFF).toByte()
        result[data.size + 3] = (checksum and 0xFF).toByte()
        return result
    }

    fun verify(dataWithCRC: ByteArray): Pair<ByteArray?, Boolean> {
        if (dataWithCRC.size < 4) return Pair(null, false)

        val data = dataWithCRC.copyOf(dataWithCRC.size - 4)
        val expected = ((dataWithCRC[dataWithCRC.size - 4].toLong() and 0xFF) shl 24) or
                       ((dataWithCRC[dataWithCRC.size - 3].toLong() and 0xFF) shl 16) or
                       ((dataWithCRC[dataWithCRC.size - 2].toLong() and 0xFF) shl 8) or
                       (dataWithCRC[dataWithCRC.size - 1].toLong() and 0xFF)
        val actual = compute(data)

        return Pair(data, actual == expected)
    }
}
