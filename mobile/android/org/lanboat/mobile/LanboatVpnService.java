package org.lanboat.mobile;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.net.VpnService;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.ParcelFileDescriptor;
import java.io.IOException;

public final class LanboatVpnService extends VpnService {
    private static volatile LanboatVpnService active;
    private volatile long request;
    private static final String CHANNEL = "lanboat-vpn";
    private static final int NOTIFICATION = 19;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        long id = intent == null ? 0 : intent.getLongExtra("request", 0);
        if (id == 0 || !LanboatBridge.isRequested(id)) {
            if (request == 0) stopSelf(startId);
            return START_NOT_STICKY;
        }
        if (request != 0 && request != id) LanboatBridge.revoked(request);
        request = id;
        active = this;
        try {
            startForeground(NOTIFICATION, notification());
            ParcelFileDescriptor tunnel = new Builder().setSession("Lanboat").setMtu(1500)
                    .addAddress("172.29.45.1", 30).addAddress("fdfe:29:45::1", 126)
                    .addRoute("0.0.0.0", 0).addRoute("::", 0)
                    .addDnsServer("172.29.45.2").addDnsServer("fdfe:29:45::2")
                    .setBlocking(false).establish();
            if (tunnel == null) throw new IllegalStateException("VPN permission revoked");
            int fd = tunnel.detachFd();
            if (!LanboatBridge.attached(id, fd)) {
                ParcelFileDescriptor.adoptFd(fd).close();
                stopSelf();
            }
        } catch (RuntimeException | IOException error) {
            LanboatBridge.attached(id, -1);
            stopSelf();
        }
        return START_NOT_STICKY;
    }

    private Notification notification() {
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        PendingIntent target = PendingIntent.getActivity(this, 0, launch, PendingIntent.FLAG_IMMUTABLE);
        Notification.Builder builder;
        if (Build.VERSION.SDK_INT >= 26) {
            getSystemService(NotificationManager.class).createNotificationChannel(
                    new NotificationChannel(CHANNEL, "Lanboat VPN", NotificationManager.IMPORTANCE_LOW));
            builder = new Notification.Builder(this, CHANNEL);
        } else {
            builder = new Notification.Builder(this);
        }
        return builder.setContentTitle("Lanboat VPN").setSmallIcon(android.R.drawable.ic_lock_lock)
                .setContentIntent(target).setOngoing(true).build();
    }

    static boolean protect(long id, int fd) {
        LanboatVpnService service = active;
        return service != null && service.request == id && service.protect(fd);
    }

    static void stop(long id) {
        new Handler(Looper.getMainLooper()).post(() -> {
            LanboatVpnService service = active;
            if (service != null && service.request == id) service.stopSelf();
        });
    }

    @Override public void onRevoke() { stopSelf(); }

    @Override public void onDestroy() {
        if (active == this) active = null;
        long id = request;
        request = 0;
        if (id != 0) LanboatBridge.revoked(id);
        stopForeground(true);
        super.onDestroy();
    }
}
