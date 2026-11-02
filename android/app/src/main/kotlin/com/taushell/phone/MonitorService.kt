package com.taushell.phone

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import java.util.concurrent.Executors

/**
 * Holds a quiet foreground notification and polls the PC so the phone can say
 * "needs an answer" or "finished" while the app is closed. LAN only: there is no
 * push service. Notifications carry the session name and state, never terminal text.
 */
class MonitorService : Service() {
    private val io = Executors.newSingleThreadExecutor()
    private val ui = Handler(Looper.getMainLooper())
    private var lastState = HashMap<String, String>()
    private var running = false
    private var failing = false

    private val tick = object : Runnable {
        override fun run() {
            if (!running) return
            io.execute {
                val pairing = Store(this@MonitorService).pairing()
                val outcome = pairing?.client()?.sessions()
                ui.post { if (running) handle(outcome) }
            }
            ui.postDelayed(this, 5000)
        }
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        channels()
        val foreground = notification("Watching your sessions", "Checking every few seconds", ongoing = true, channel = WATCH)
        startForeground(1, foreground, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        if (!running) {
            running = true
            ui.post(tick)
        }
        return START_STICKY
    }

    override fun onDestroy() {
        running = false
        ui.removeCallbacksAndMessages(null)
        io.shutdownNow()
        super.onDestroy()
    }

    private fun handle(outcome: Result<List<SessionItem>>?) {
        val manager = getSystemService(NotificationManager::class.java)
        when (outcome) {
            null -> stopSelf()
            is Result.Fail -> {
                if (!failing) {
                    failing = true
                    manager.notify(2, notification("Cannot reach your PC", outcome.message, ongoing = false, channel = WATCH))
                }
            }
            is Result.Ok -> {
                if (failing) {
                    failing = false
                    manager.cancel(2)
                }
                val first = lastState.isEmpty()
                val next = HashMap<String, String>()
                outcome.value.forEachIndexed { index, item ->
                    // Only confirmed readings count, so a guess never pages anyone.
                    val state = if (item.running && item.activity.confirmed) item.activity.state else "other"
                    next[item.id] = state
                    val before = lastState[item.id]
                    if (!first && before != null && before != state) {
                        when {
                            state == "input" -> manager.notify(100 + index, notification(item.name, "Needs an answer", ongoing = false, channel = ALERT))
                            state == "idle" && before == "working" -> manager.notify(100 + index, notification(item.name, "Finished", ongoing = false, channel = ALERT))
                        }
                    }
                }
                lastState = next
            }
        }
    }

    private fun channels() {
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(WATCH, "Watching", NotificationManager.IMPORTANCE_MIN))
        manager.createNotificationChannel(NotificationChannel(ALERT, "Session alerts", NotificationManager.IMPORTANCE_HIGH))
    }

    private fun notification(title: String, body: String, ongoing: Boolean, channel: String): Notification {
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        return Notification.Builder(this, channel)
            .setSmallIcon(R.drawable.ic_stat)
            .setContentTitle(title)
            .setContentText(body)
            .setContentIntent(open)
            .setOngoing(ongoing)
            .setAutoCancel(!ongoing)
            .build()
    }

    companion object {
        private const val WATCH = "watch"
        private const val ALERT = "alert"

        fun start(context: Context) {
            context.startForegroundService(Intent(context, MonitorService::class.java))
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, MonitorService::class.java))
        }
    }
}
