#import <Foundation/Foundation.h>
#import <NetworkExtension/NetworkExtension.h>
#include <stdint.h>
#include <sys/socket.h>

extern void LanboatTunnelStart(int64_t, void *, int);
extern void LanboatTunnelStop(int64_t);
extern void LanboatTunnelPacket(int64_t, void *, int);
extern void LanboatTunnelMessage(int64_t, int64_t, void *, int);

typedef void (^StartHandler)(NSError *);
typedef void (^StopHandler)(void);
typedef void (^MessageHandler)(NSData *);

@interface LanboatPacketTunnel : NEPacketTunnelProvider
@property(atomic) int64_t generation;
@property(nonatomic, copy) StartHandler started;
@property(nonatomic, copy) StopHandler stopped;
@property(nonatomic, strong) NSMutableDictionary<NSNumber *, MessageHandler> *messages;
- (void)readPackets:(int64_t)generation;
@end

static LanboatPacketTunnel *provider;
static int64_t sequence;

static LanboatPacketTunnel *getProvider(void) {
    @synchronized (LanboatPacketTunnel.class) { return provider; }
}

static NSError *failure(NSString *message) {
    return [NSError errorWithDomain:@"Lanboat.VPN" code:1
                           userInfo:@{NSLocalizedDescriptionKey: message}];
}

@implementation LanboatPacketTunnel
- (void)startTunnelWithOptions:(NSDictionary<NSString *, NSObject *> *)options
            completionHandler:(StartHandler)complete {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSData *config = (NSData *)options[@"lanboat"];
        if (![config isKindOfClass:NSData.class] || config.length == 0 || config.length > (1 << 20)) {
            complete(failure(@"Missing Lanboat VPN configuration. Start VPN from the app."));
            return;
        }
        @synchronized (LanboatPacketTunnel.class) { provider = self; }
        int64_t generation = ++sequence;
        self.generation = generation;
        self.started = complete;
        self.messages = [NSMutableDictionary new];
        NEPacketTunnelNetworkSettings *settings = [[NEPacketTunnelNetworkSettings alloc] initWithTunnelRemoteAddress:@"172.29.45.2"];
        settings.MTU = @1500;
        settings.IPv4Settings = [[NEIPv4Settings alloc] initWithAddresses:@[@"172.29.45.1"] subnetMasks:@[@"255.255.255.252"]];
        settings.IPv4Settings.includedRoutes = @[[NEIPv4Route defaultRoute]];
        settings.IPv6Settings = [[NEIPv6Settings alloc] initWithAddresses:@[@"fdfe:29:45::1"] networkPrefixLengths:@[@126]];
        settings.IPv6Settings.includedRoutes = @[[NEIPv6Route defaultRoute]];
        settings.DNSSettings = [[NEDNSSettings alloc] initWithServers:@[@"1.1.1.1"]];
        settings.DNSSettings.matchDomains = @[@""];
        [self setTunnelNetworkSettings:settings completionHandler:^(NSError *error) {
            dispatch_async(dispatch_get_main_queue(), ^{
                if (self.generation != generation) return;
                if (error) {
                    StartHandler handler = self.started;
                    self.started = nil;
                    handler(error);
                    return;
                }
                LanboatTunnelStart(generation, (void *)config.bytes, (int)config.length);
            });
        }];
    });
}

- (void)readPackets:(int64_t)generation {
    [self.packetFlow readPacketsWithCompletionHandler:^(NSArray<NSData *> *packets, NSArray<NSNumber *> *protocols) {
        if (self.generation != generation) return;
        for (NSData *packet in packets) {
            if (packet.length <= 65575) LanboatTunnelPacket(generation, (void *)packet.bytes, (int)packet.length);
        }
        if (self.generation == generation) [self readPackets:generation];
    }];
}

- (void)stopTunnelWithReason:(NEProviderStopReason)reason completionHandler:(StopHandler)complete {
    dispatch_async(dispatch_get_main_queue(), ^{
        int64_t generation = self.generation;
        self.generation = 0;
        self.stopped = complete;
        if (self.started) {
            StartHandler handler = self.started;
            self.started = nil;
            handler(failure(@"VPN stopped during startup"));
        }
        for (MessageHandler handler in self.messages.allValues) handler(nil);
        [self.messages removeAllObjects];
        LanboatTunnelStop(generation);
    });
}

- (void)handleAppMessage:(NSData *)message completionHandler:(MessageHandler)complete {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!complete) return;
        if (self.generation == 0 || message.length > (1 << 20)) { complete(nil); return; }
        int64_t ticket = ++sequence;
        self.messages[@(ticket)] = [complete copy];
        LanboatTunnelMessage(self.generation, ticket, (void *)message.bytes, (int)message.length);
    });
}
@end

int lbTunnelWrite(int64_t generation, const void *packet, int size) {
    @autoreleasepool {
        LanboatPacketTunnel *active = getProvider();
        if (!active || active.generation != generation || size <= 0) return 0;
        const uint8_t version = ((const uint8_t *)packet)[0] >> 4;
        if (version != 4 && version != 6) return 0;
        NSData *data = [NSData dataWithBytes:packet length:(NSUInteger)size];
        return [active.packetFlow writePackets:@[data] withProtocols:@[@(version == 4 ? AF_INET : AF_INET6)]];
    }
}

void lbTunnelStarted(int64_t generation, const char *errorText) {
    @autoreleasepool {
        NSString *message = errorText ? [NSString stringWithUTF8String:errorText] : nil;
        dispatch_async(dispatch_get_main_queue(), ^{
            LanboatPacketTunnel *active = getProvider();
            if (!active || active.generation != generation || !active.started) return;
            StartHandler complete = active.started;
            active.started = nil;
            if (!message) [active readPackets:generation];
            complete(message ? failure(message) : nil);
        });
    }
}

void lbTunnelFailed(int64_t generation, const char *errorText) {
    @autoreleasepool {
        NSString *message = [NSString stringWithUTF8String:errorText];
        dispatch_async(dispatch_get_main_queue(), ^{
            LanboatPacketTunnel *active = getProvider();
            if (active.generation == generation) [active cancelTunnelWithError:failure(message)];
        });
    }
}

void lbTunnelStopped(int64_t generation) {
    dispatch_async(dispatch_get_main_queue(), ^{
        LanboatPacketTunnel *active = getProvider();
        if (!active || active.generation != 0 || !active.stopped) return;
        StopHandler complete = active.stopped;
        active.stopped = nil;
        @synchronized (LanboatPacketTunnel.class) { provider = nil; }
        complete();
    });
}

void lbTunnelReply(int64_t ticket, const void *data, int size) {
    @autoreleasepool {
        NSData *reply = [NSData dataWithBytes:data length:(NSUInteger)size];
        dispatch_async(dispatch_get_main_queue(), ^{
            LanboatPacketTunnel *active = getProvider();
            MessageHandler complete = active.messages[@(ticket)];
            [active.messages removeObjectForKey:@(ticket)];
            if (complete) complete(reply);
        });
    }
}
