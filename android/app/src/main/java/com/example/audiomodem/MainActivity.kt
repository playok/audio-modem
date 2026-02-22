package com.example.audiomodem

import android.Manifest
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Bundle
import android.provider.OpenableColumns
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.audiomodem.modem.Modulation
import com.example.audiomodem.viewmodel.*

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            AudioModemTheme {
                AudioModemApp()
            }
        }
    }
}

@Composable
fun AudioModemTheme(content: @Composable () -> Unit) {
    val darkColors = darkColorScheme(
        primary = Color(0xFF00D4FF),
        onPrimary = Color.White,
        surface = Color(0xFF1A1A2E),
        background = Color(0xFF0F0F23),
        onSurface = Color(0xFFE0E0E0),
        onBackground = Color(0xFFE0E0E0),
    )
    MaterialTheme(colorScheme = darkColors, content = content)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AudioModemApp(vm: MainViewModel = viewModel()) {
    val state by vm.state.collectAsState()
    val context = LocalContext.current

    // Permission launcher
    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { }

    LaunchedEffect(Unit) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO)
            != PackageManager.PERMISSION_GRANTED) {
            permissionLauncher.launch(Manifest.permission.RECORD_AUDIO)
        }
    }

    // File picker
    val filePicker = rememberLauncherForActivityResult(
        ActivityResultContracts.GetContent()
    ) { uri: Uri? ->
        uri?.let {
            val cursor = context.contentResolver.query(it, null, null, null, null)
            val name = cursor?.use { c ->
                val idx = c.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                c.moveToFirst()
                if (idx >= 0) c.getString(idx) else "unknown"
            } ?: "unknown"
            vm.selectFile(it, name)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("Audio Modem", fontWeight = FontWeight.Bold)
                        Text("3.5mm AUX 파일 전송",
                            fontSize = 12.sp,
                            color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.6f))
                    }
                },
                actions = {
                    StatusIndicator(state.status)
                }
            )
        }
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            contentPadding = PaddingValues(vertical = 12.dp)
        ) {
            // Mode selector
            item {
                Card {
                    Column(Modifier.padding(16.dp)) {
                        Text("전송 모드", style = MaterialTheme.typography.titleMedium,
                            color = MaterialTheme.colorScheme.primary)
                        Spacer(Modifier.height(12.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            ModeButton("파일 전송", Icons.Default.Upload,
                                state.mode == AppMode.SEND,
                                Modifier.weight(1f)) { vm.setMode(AppMode.SEND) }
                            ModeButton("파일 수신", Icons.Default.Download,
                                state.mode == AppMode.RECEIVE,
                                Modifier.weight(1f)) { vm.setMode(AppMode.RECEIVE) }
                        }
                    }
                }
            }

            // Modulation selector
            item {
                Card {
                    Column(Modifier.padding(16.dp)) {
                        Text("변조 방식", style = MaterialTheme.typography.titleMedium,
                            color = MaterialTheme.colorScheme.primary)
                        Spacer(Modifier.height(8.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Modulation.entries.forEach { mod ->
                                FilterChip(
                                    selected = state.modulation == mod,
                                    onClick = { vm.setModulation(mod) },
                                    label = { Text(mod.toString()) }
                                )
                            }
                        }
                    }
                }
            }

            // Send/Receive panel
            item {
                if (state.mode == AppMode.SEND) {
                    SendPanel(state, onPickFile = { filePicker.launch("*/*") },
                        onSend = { vm.startSend() })
                } else {
                    ReceivePanel(state, onReceive = { vm.startReceive() })
                }
            }

            // Progress
            if (state.status == TransferStatus.TRANSFERRING || state.status == TransferStatus.COMPLETED) {
                item { ProgressPanel(state) }
            }

            // Logs
            item { LogPanel(state.logs) }
        }
    }
}

@Composable
fun StatusIndicator(status: TransferStatus) {
    val (color, label) = when (status) {
        TransferStatus.IDLE -> Color.Gray to "대기"
        TransferStatus.CONNECTING -> Color(0xFFFFAA00) to "연결 중"
        TransferStatus.TRANSFERRING -> Color(0xFF00D4FF) to "전송 중"
        TransferStatus.COMPLETED -> Color(0xFF00FF88) to "완료"
        TransferStatus.ERROR -> Color(0xFFFF4444) to "오류"
    }

    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .background(
                MaterialTheme.colorScheme.surface,
                RoundedCornerShape(20.dp)
            )
            .padding(horizontal = 12.dp, vertical = 6.dp)
    ) {
        Box(
            modifier = Modifier
                .size(8.dp)
                .clip(CircleShape)
                .background(color)
        )
        Spacer(Modifier.width(8.dp))
        Text(label, fontSize = 13.sp)
    }
}

@Composable
fun ModeButton(
    label: String,
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    selected: Boolean,
    modifier: Modifier = Modifier,
    onClick: () -> Unit
) {
    val borderColor = if (selected) MaterialTheme.colorScheme.primary else Color(0xFF2A2A4A)
    val bgColor = if (selected) MaterialTheme.colorScheme.primary.copy(alpha = 0.1f) else Color.Transparent

    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier = modifier
            .border(2.dp, borderColor, RoundedCornerShape(10.dp))
            .background(bgColor, RoundedCornerShape(10.dp))
            .clickable(onClick = onClick)
            .padding(16.dp)
    ) {
        Icon(icon, contentDescription = label,
            tint = if (selected) MaterialTheme.colorScheme.primary else Color.Gray)
        Spacer(Modifier.height(4.dp))
        Text(label, color = if (selected) MaterialTheme.colorScheme.primary else Color.Gray)
    }
}

@Composable
fun SendPanel(state: TransferState, onPickFile: () -> Unit, onSend: () -> Unit) {
    Card {
        Column(Modifier.padding(16.dp)) {
            Text("파일 전송", style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(12.dp))

            // File picker area
            Box(
                contentAlignment = Alignment.Center,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(100.dp)
                    .border(2.dp, Color(0xFF2A2A4A), RoundedCornerShape(10.dp))
                    .clickable(onClick = onPickFile)
                    .padding(16.dp)
            ) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Icon(Icons.Default.Folder, contentDescription = "파일 선택",
                        tint = Color.Gray, modifier = Modifier.size(32.dp))
                    Text(
                        if (state.selectedFileName.isNotEmpty()) state.selectedFileName
                        else "파일을 선택하세요",
                        color = if (state.selectedFileName.isNotEmpty())
                            MaterialTheme.colorScheme.primary else Color.Gray,
                        fontSize = 14.sp
                    )
                }
            }

            Spacer(Modifier.height(12.dp))

            Button(
                onClick = onSend,
                enabled = state.selectedFileUri != null && state.status == TransferStatus.IDLE,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(10.dp)
            ) {
                Text("전송 시작", modifier = Modifier.padding(vertical = 4.dp))
            }
        }
    }
}

@Composable
fun ReceivePanel(state: TransferState, onReceive: () -> Unit) {
    Card {
        Column(Modifier.padding(16.dp)) {
            Text("파일 수신", style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(8.dp))
            Text("AUX 케이블을 연결하고 수신 대기를 시작하세요.",
                color = Color.Gray, fontSize = 14.sp)
            Spacer(Modifier.height(12.dp))

            Button(
                onClick = onReceive,
                enabled = state.status == TransferStatus.IDLE,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(10.dp)
            ) {
                Text("수신 대기", modifier = Modifier.padding(vertical = 4.dp))
            }
        }
    }
}

@Composable
fun ProgressPanel(state: TransferState) {
    Card {
        Column(Modifier.padding(16.dp)) {
            Text("전송 진행", style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(12.dp))

            LinearProgressIndicator(
                progress = { state.progress },
                modifier = Modifier
                    .fillMaxWidth()
                    .height(8.dp)
                    .clip(RoundedCornerShape(4.dp)),
            )

            Spacer(Modifier.height(8.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text("${(state.progress * 100).toInt()}%", fontSize = 13.sp, color = Color.Gray)
                if (state.totalBytes > 0) {
                    Text("${formatSize(state.bytesSent)} / ${formatSize(state.totalBytes)}",
                        fontSize = 13.sp, color = Color.Gray)
                }
            }

            if (state.message.isNotEmpty()) {
                Text(state.message, fontSize = 13.sp, color = Color.Gray,
                    modifier = Modifier.padding(top = 4.dp))
            }
        }
    }
}

@Composable
fun LogPanel(logs: List<String>) {
    Card {
        Column(Modifier.padding(16.dp)) {
            Text("로그", style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(8.dp))

            val listState = rememberLazyListState()

            LaunchedEffect(logs.size) {
                if (logs.isNotEmpty()) listState.animateScrollToItem(logs.size - 1)
            }

            LazyColumn(
                state = listState,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(150.dp)
            ) {
                items(logs) { log ->
                    Text(
                        log,
                        fontSize = 11.sp,
                        fontFamily = FontFamily.Monospace,
                        color = when {
                            "오류" in log || "실패" in log -> Color(0xFFFF4444)
                            "완료" in log -> Color(0xFF00FF88)
                            "연결" in log || "대기" in log -> Color(0xFFFFAA00)
                            else -> Color(0xFF888888)
                        },
                        modifier = Modifier.padding(vertical = 1.dp)
                    )
                }
            }
        }
    }
}

fun formatSize(bytes: Long): String {
    if (bytes == 0L) return "0 B"
    val units = arrayOf("B", "KB", "MB", "GB")
    val i = (Math.log(bytes.toDouble()) / Math.log(1024.0)).toInt().coerceAtMost(units.size - 1)
    val size = bytes / Math.pow(1024.0, i.toDouble())
    return "%.1f %s".format(size, units[i])
}
