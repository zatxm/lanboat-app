//go:build ios && cgo

#import <Foundation/Foundation.h>
#import <NetworkExtension/NetworkExtension.h>
#import <UIKit/UIKit.h>
#include <stdint.h>

extern void LanboatIOSEvent(int64_t, int, char *);
extern void LanboatIOSReply(int64_t, void *, int);

static NETunnelProviderManager *manager;
static id observer;
static int64_t current;
static BOOL started;
static BOOL connected;
static NSData *pendingConfig;

static void sendEvent(int64_t id, int event, NSString *text) {
    LanboatIOSEvent(id, event, (char *)text.UTF8String);
}

static void removeObserver(void) {
    if (observer) [[NSNotificationCenter defaultCenter] removeObserver:observer];
    observer = nil;
}

static void fail(int64_t id, NSError *error) {
    if (current != id) return;
    current = 0;
    pendingConfig = nil;
    removeObserver();
    sendEvent(id, -1, error.localizedDescription ?: @"无法启动 iOS VPN");
}

static void startTunnel(int64_t id) {
    if (current != id || started || !pendingConfig) return;
    NETunnelProviderSession *session = (NETunnelProviderSession *)manager.connection;
    if (session.status != NEVPNStatusDisconnected && session.status != NEVPNStatusInvalid) return;
    started = YES;
    NSError *error = nil;
    NSData *config = pendingConfig;
    pendingConfig = nil;
    if (![session startTunnelWithOptions:@{@"lanboat": config} andReturnError:&error]) {
        fail(id, error);
    }
}

static void attachManager(int64_t id) {
    if (current != id) return;
    removeObserver();
    observer = [[NSNotificationCenter defaultCenter]
        addObserverForName:NEVPNStatusDidChangeNotification object:manager.connection queue:[NSOperationQueue mainQueue]
        usingBlock:^(NSNotification *note) {
            if (current != id) return;
            switch (manager.connection.status) {
                case NEVPNStatusConnected:
                    if (started && !connected) {
                        connected = YES;
                        sendEvent(id, 1, @"");
                    }
                    break;
                case NEVPNStatusDisconnected:
                case NEVPNStatusInvalid:
                    if (!started) {
                        startTunnel(id);
                    } else {
                        current = 0;
                        removeObserver();
                        sendEvent(id, connected ? 0 : -1, @"iOS VPN 扩展未能启动");
                    }
                    break;
                default: break;
            }
        }];
    if (manager.connection.status == NEVPNStatusDisconnected || manager.connection.status == NEVPNStatusInvalid) {
        startTunnel(id);
    } else {
        [manager.connection stopVPNTunnel];
    }
}

void lbIOSStart(int64_t id, const void *data, int length) {
    @autoreleasepool {
        NSData *config = [NSData dataWithBytes:data length:(NSUInteger)length];
        dispatch_async(dispatch_get_main_queue(), ^{
            removeObserver();
            current = id;
            pendingConfig = config;
            started = NO;
            connected = NO;
            NSString *providerID = [NSBundle.mainBundle.bundleIdentifier stringByAppendingString:@".PacketTunnel"];
            [NETunnelProviderManager loadAllFromPreferencesWithCompletionHandler:^(NSArray<NETunnelProviderManager *> *items, NSError *error) {
                dispatch_async(dispatch_get_main_queue(), ^{
                    if (current != id) return;
                    if (error) { fail(id, error); return; }
                    manager = nil;
                    for (NETunnelProviderManager *item in items) {
                        NEVPNProtocol *configuration = item.protocolConfiguration;
                        if ([configuration isKindOfClass:NETunnelProviderProtocol.class] &&
                            [((NETunnelProviderProtocol *)configuration).providerBundleIdentifier isEqualToString:providerID]) {
                            manager = item;
                            break;
                        }
                    }
                    if (manager && manager.enabled) { attachManager(id); return; }
                    if (!manager) manager = [NETunnelProviderManager new];
                    NETunnelProviderProtocol *protocol = [NETunnelProviderProtocol new];
                    protocol.providerBundleIdentifier = providerID;
                    protocol.serverAddress = @"Lanboat WireGuard";
                    manager.protocolConfiguration = protocol;
                    manager.localizedDescription = @"Lanboat";
                    manager.enabled = YES;
                    [manager saveToPreferencesWithCompletionHandler:^(NSError *saveError) {
                        dispatch_async(dispatch_get_main_queue(), ^{
                            if (current != id) return;
                            if (saveError) { fail(id, saveError); return; }
                            [manager loadFromPreferencesWithCompletionHandler:^(NSError *loadError) {
                                dispatch_async(dispatch_get_main_queue(), ^{
                                    if (current != id) return;
                                    if (loadError) fail(id, loadError);
                                    else attachManager(id);
                                });
                            }];
                        });
                    }];
                });
            }];
        });
    }
}

void lbIOSStop(int64_t id) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (current != id) return;
        current = 0;
        pendingConfig = nil;
        removeObserver();
        [manager.connection stopVPNTunnel];
    });
}

void lbIOSMessage(int64_t id, int64_t ticket, const void *data, int length) {
    @autoreleasepool {
        NSData *message = [NSData dataWithBytes:data length:(NSUInteger)length];
        dispatch_async(dispatch_get_main_queue(), ^{
            if (current != id || !connected) { LanboatIOSReply(ticket, NULL, 0); return; }
            NSError *error = nil;
            BOOL accepted = [(NETunnelProviderSession *)manager.connection
                sendProviderMessage:message returnError:&error
                responseHandler:^(NSData *reply) {
                    LanboatIOSReply(ticket, (void *)reply.bytes, (int)reply.length);
                }];
            if (!accepted) LanboatIOSReply(ticket, NULL, 0);
        });
    }
}

int lbIOSOpen(const char *url) {
    @autoreleasepool {
        NSURL *target = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
        if (![@[@"http", @"https"] containsObject:target.scheme.lowercaseString]) return 0;
        dispatch_async(dispatch_get_main_queue(), ^{
            [UIApplication.sharedApplication openURL:target options:@{} completionHandler:nil];
        });
        return 1;
    }
}

#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>

extern void LanboatFileResult(char *);

@interface LanboatFiles : NSObject<UIDocumentPickerDelegate>
@property(nonatomic, assign) BOOL exporting;
@end

@implementation LanboatFiles
- (void)documentPickerWasCancelled:(UIDocumentPickerViewController *)controller {
    LanboatFileResult("");
}

- (void)documentPicker:(UIDocumentPickerViewController *)controller didPickDocumentsAtURLs:(NSArray<NSURL *> *)urls {
    if (self.exporting) {
        LanboatFileResult("");
        return;
    }
    NSURL *source = urls.firstObject;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        NSNumber *length = nil;
        NSError *error = nil;
        if (!source || ![source getResourceValue:&length forKey:NSURLFileSizeKey error:&error] ||
            length.unsignedLongLongValue > (512ULL << 20)) {
            LanboatFileResult("-");
            return;
        }
        NSURL *dir = [[NSURL fileURLWithPath:NSTemporaryDirectory() isDirectory:YES]
            URLByAppendingPathComponent:[@"lanboat-import-" stringByAppendingString:NSUUID.UUID.UUIDString] isDirectory:YES];
        NSURL *target = [dir URLByAppendingPathComponent:source.lastPathComponent];
        NSFileManager *files = NSFileManager.defaultManager;
        if (![files createDirectoryAtURL:dir withIntermediateDirectories:YES attributes:nil error:&error] ||
            ![files copyItemAtURL:source toURL:target error:&error]) {
            [files removeItemAtURL:dir error:nil];
            LanboatFileResult("-");
            return;
        }
        LanboatFileResult((char *)target.path.UTF8String);
    });
}
@end

static LanboatFiles *filesDelegate;

static int chooseDocument(NSURL *target) {
    dispatch_async(dispatch_get_main_queue(), ^{
        UIWindow *window = nil;
        for (UIScene *scene in UIApplication.sharedApplication.connectedScenes) {
            if ([scene isKindOfClass:UIWindowScene.class] && scene.activationState == UISceneActivationStateForegroundActive) {
                for (UIWindow *candidate in ((UIWindowScene *)scene).windows) {
                    if (candidate.isKeyWindow) { window = candidate; break; }
                }
            }
        }
        UIViewController *presenter = window.rootViewController;
        while (presenter.presentedViewController) presenter = presenter.presentedViewController;
        if (!presenter) {
            LanboatFileResult("-");
            return;
        }
        UIDocumentPickerViewController *picker = target
            ? [[UIDocumentPickerViewController alloc] initForExportingURLs:@[target] asCopy:YES]
            : [[UIDocumentPickerViewController alloc] initForOpeningContentTypes:@[UTTypeData] asCopy:YES];
        filesDelegate = [LanboatFiles new];
        filesDelegate.exporting = target != nil;
        picker.delegate = filesDelegate;
        [presenter presentViewController:picker animated:YES completion:nil];
    });
    return 1;
}

int lbPickFile(void) { return chooseDocument(nil); }

int lbSaveFile(const char *path) {
    @autoreleasepool {
        return chooseDocument([NSURL fileURLWithPath:[NSString stringWithUTF8String:path]]);
    }
}
