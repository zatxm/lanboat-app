package org.lanboat.mobile;

import android.app.Activity;
import android.content.Context;
import android.database.Cursor;
import android.content.Intent;
import android.net.Uri;
import android.net.VpnService;
import android.os.Build;
import android.provider.OpenableColumns;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.lang.ref.WeakReference;
import java.util.Locale;
import java.util.TimeZone;

public final class LanboatBridge {
    private static final int VPN_PERMISSION = 118, PICK_FILE = 119, SAVE_FILE = 120;
    private static volatile WeakReference<Activity> activity = new WeakReference<>(null);
    private static volatile Context context;
    private static volatile long requested;
    private static long awaitingPermission;
    private static String exportPath;

    public static void attach(Activity host) {
        activity = new WeakReference<>(host);
        context = host.getApplicationContext();
        init(context.getFilesDir().getAbsolutePath(),
                TimeZone.getDefault().getID(), Locale.getDefault().toLanguageTag());
    }

    private static native void init(String home, String zone, String language);
    static native boolean attached(long request, int fd);
    static native void revoked(long request);
    private static native void fileResult(String path);

    public static boolean startRequest(long request) {
        Activity host = activity.get();
        if (host == null || host.isFinishing()) return false;
        requested = request;
        host.runOnUiThread(() -> {
            if (request != requested) return;
            try {
                Intent prepare = VpnService.prepare(host);
                if (prepare == null) {
                    startService(request);
                } else {
                    awaitingPermission = request;
                    host.startActivityForResult(prepare, VPN_PERMISSION);
                }
            } catch (RuntimeException error) {
                attached(request, -1);
            }
        });
        return true;
    }

    public static void activityResult(int code, int result, Intent data) {
        if (code == PICK_FILE || code == SAVE_FILE) {
            String source = exportPath;
            exportPath = null;
            if (result != Activity.RESULT_OK || data == null || data.getData() == null) {
                fileResult("");
            } else {
                Uri uri = data.getData();
                new Thread(() -> transferFile(uri, code == SAVE_FILE ? source : null), "Lanboat file transfer").start();
            }
            return;
        }
        if (code != VPN_PERMISSION) return;
        long request = awaitingPermission;
        awaitingPermission = 0;
        if (request == 0 || request != requested) return;
        if (result == Activity.RESULT_OK) startService(request);
        else attached(request, -1);
    }

    private static void startService(long request) {
        Intent intent = new Intent(context, LanboatVpnService.class).putExtra("request", request);
        try {
            if (Build.VERSION.SDK_INT >= 26) context.startForegroundService(intent);
            else context.startService(intent);
        } catch (RuntimeException error) {
            attached(request, -1);
        }
    }

    public static boolean stopRequest(long request) {
        if (requested == request) requested = 0;
        LanboatVpnService.stop(request);
        return true;
    }

    static boolean isRequested(long request) { return requested == request; }

    public static boolean protectSocket(long request, int fd) {
        return LanboatVpnService.protect(request, fd);
    }

    public static boolean chooseFile(String path) {
        Activity host = activity.get();
        if (host == null || host.isFinishing()) return false;
        host.runOnUiThread(() -> {
            exportPath = path;
            Intent intent = new Intent(path == null ? Intent.ACTION_OPEN_DOCUMENT : Intent.ACTION_CREATE_DOCUMENT);
            intent.addCategory(Intent.CATEGORY_OPENABLE);
            intent.setType(path == null ? "*/*" : "application/octet-stream");
            if (path != null) intent.putExtra(Intent.EXTRA_TITLE, new File(path).getName());
            try {
                host.startActivityForResult(intent, path == null ? PICK_FILE : SAVE_FILE);
            } catch (RuntimeException error) {
                fileResult("-");
            }
        });
        return true;
    }

    private static void transferFile(Uri uri, String export) {
        File imported = null;
        try {
            if (export != null) {
                try (InputStream in = new FileInputStream(export);
                     OutputStream out = context.getContentResolver().openOutputStream(uri, "w")) {
                    transfer(in, out, Long.MAX_VALUE);
                }
                fileResult("");
                return;
            }
            String name = "import";
            try (Cursor cursor = context.getContentResolver().query(uri, new String[]{OpenableColumns.DISPLAY_NAME}, null, null, null)) {
                if (cursor != null && cursor.moveToFirst()) {
                    String supplied = cursor.getString(0);
                    if (supplied != null) name = new File(supplied).getName();
                }
            }
            if (name.isEmpty() || name.equals(".") || name.equals("..")) name = "import";
            File dir = File.createTempFile("import-", ".dir", context.getCacheDir());
            if (!dir.delete() || !dir.mkdir()) throw new IOException("Cannot create import directory");
            imported = new File(dir, name);
            try (InputStream in = context.getContentResolver().openInputStream(uri);
                 OutputStream out = new FileOutputStream(imported)) {
                transfer(in, out, 512L << 20);
            }
            fileResult(imported.getAbsolutePath());
        } catch (IOException | RuntimeException error) {
            if (imported != null) {
                imported.delete();
                imported.getParentFile().delete();
            }
            fileResult("-");
        }
    }

    private static void transfer(InputStream in, OutputStream out, long limit) throws IOException {
        if (in == null || out == null) throw new IOException("File is unavailable");
        byte[] buffer = new byte[65536];
        for (int n; (n = in.read(buffer)) != -1; ) {
            limit -= n;
            if (limit < 0) throw new IOException("Import is too large");
            out.write(buffer, 0, n);
        }
    }

    public static boolean openURL(String url) {
        Activity host = activity.get();
        Uri uri = Uri.parse(url);
        if (host == null || (!"http".equals(uri.getScheme()) && !"https".equals(uri.getScheme()))) return false;
        host.runOnUiThread(() -> {
            try {
                host.startActivity(new Intent(Intent.ACTION_VIEW, uri));
            } catch (RuntimeException error) {
                android.util.Log.w("Lanboat", "Cannot open a browser", error);
            }
        });
        return true;
    }
}
