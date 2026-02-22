package com.example.audiomodem.viewmodel

import android.app.Application
import android.net.Uri
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.audiomodem.audio.AudioIO
import com.example.audiomodem.modem.Modulation
import com.example.audiomodem.protocol.FileTransfer
import com.example.audiomodem.protocol.Transport
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import java.io.File

enum class AppMode { SEND, RECEIVE }
enum class TransferStatus { IDLE, CONNECTING, TRANSFERRING, COMPLETED, ERROR }

data class TransferState(
    val status: TransferStatus = TransferStatus.IDLE,
    val mode: AppMode = AppMode.SEND,
    val modulation: Modulation = Modulation.QAM16,
    val progress: Float = 0f,
    val bytesSent: Long = 0,
    val totalBytes: Long = 0,
    val message: String = "",
    val selectedFileName: String = "",
    val selectedFileUri: Uri? = null,
    val logs: List<String> = emptyList()
)

class MainViewModel(application: Application) : AndroidViewModel(application) {

    private val _state = MutableStateFlow(TransferState())
    val state: StateFlow<TransferState> = _state

    private var audioIO: AudioIO? = null
    private var transport: Transport? = null

    fun setMode(mode: AppMode) {
        _state.value = _state.value.copy(mode = mode)
    }

    fun setModulation(mod: Modulation) {
        _state.value = _state.value.copy(modulation = mod)
    }

    fun selectFile(uri: Uri, filename: String) {
        _state.value = _state.value.copy(
            selectedFileUri = uri,
            selectedFileName = filename
        )
        addLog("파일 선택: $filename")
    }

    fun startSend() {
        val uri = _state.value.selectedFileUri ?: return
        val filename = _state.value.selectedFileName

        viewModelScope.launch(Dispatchers.IO) {
            try {
                updateStatus(TransferStatus.CONNECTING, "오디오 장치 초기화...")
                initAudio()

                updateStatus(TransferStatus.CONNECTING, "핸드셰이크 중...")
                if (!transport!!.handshake()) {
                    updateStatus(TransferStatus.ERROR, "핸드셰이크 실패")
                    return@launch
                }

                updateStatus(TransferStatus.TRANSFERRING, "파일 전송 중...")
                val ft = FileTransfer(transport!!)
                ft.onProgress = { sent, total, msg ->
                    val progress = if (total > 0) sent.toFloat() / total else 0f
                    _state.value = _state.value.copy(
                        progress = progress,
                        bytesSent = sent,
                        totalBytes = total,
                        message = msg
                    )
                }

                val context = getApplication<Application>()
                if (ft.sendFile(context, uri, filename)) {
                    updateStatus(TransferStatus.COMPLETED, "전송 완료!")
                } else {
                    updateStatus(TransferStatus.ERROR, "전송 실패")
                }
            } catch (e: Exception) {
                updateStatus(TransferStatus.ERROR, "오류: ${e.message}")
            } finally {
                releaseAudio()
            }
        }
    }

    fun startReceive() {
        viewModelScope.launch(Dispatchers.IO) {
            try {
                updateStatus(TransferStatus.CONNECTING, "오디오 장치 초기화...")
                initAudio()

                updateStatus(TransferStatus.CONNECTING, "핸드셰이크 대기 중...")
                if (!transport!!.waitForHandshake(30000)) {
                    updateStatus(TransferStatus.ERROR, "핸드셰이크 타임아웃")
                    return@launch
                }

                updateStatus(TransferStatus.TRANSFERRING, "파일 수신 중...")
                val outputDir = File(getApplication<Application>().filesDir, "received")
                val ft = FileTransfer(transport!!)
                ft.onProgress = { received, total, msg ->
                    val progress = if (total > 0) received.toFloat() / total else 0f
                    _state.value = _state.value.copy(
                        progress = progress,
                        bytesSent = received,
                        totalBytes = total,
                        message = msg
                    )
                }

                val meta = ft.receiveFile(outputDir, 60000)
                if (meta != null) {
                    updateStatus(TransferStatus.COMPLETED,
                        "수신 완료: ${meta.filename} (${meta.size} bytes)")
                } else {
                    updateStatus(TransferStatus.ERROR, "수신 실패")
                }
            } catch (e: Exception) {
                updateStatus(TransferStatus.ERROR, "오류: ${e.message}")
            } finally {
                releaseAudio()
            }
        }
    }

    private fun initAudio() {
        audioIO = AudioIO().also {
            it.openInput()
            it.openOutput()
        }
        transport = Transport(audioIO!!, _state.value.modulation)
    }

    private fun releaseAudio() {
        audioIO?.close()
        audioIO = null
        transport = null
    }

    private fun updateStatus(status: TransferStatus, message: String) {
        addLog(message)
        _state.value = _state.value.copy(status = status, message = message)
    }

    private fun addLog(message: String) {
        val timestamp = java.text.SimpleDateFormat("HH:mm:ss", java.util.Locale.getDefault())
            .format(java.util.Date())
        val logs = _state.value.logs + "[$timestamp] $message"
        _state.value = _state.value.copy(logs = logs.takeLast(50))
    }

    override fun onCleared() {
        super.onCleared()
        releaseAudio()
    }
}
