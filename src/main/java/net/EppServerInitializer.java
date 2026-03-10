package net;

import io.netty.channel.ChannelInitializer;
import io.netty.channel.socket.SocketChannel;
import io.netty.handler.ssl.SslContext;
import io.netty.handler.ssl.SslContextBuilder;
import io.netty.handler.timeout.IdleStateHandler;
import io.netty.handler.logging.LoggingHandler;
import io.netty.handler.logging.LogLevel;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.ChannelDuplexHandler;
import io.netty.handler.timeout.IdleStateEvent;
import io.netty.channel.ChannelInboundHandlerAdapter;
import io.netty.handler.ssl.ClientAuth;
import util.XmlUtils;
import session.SessionManager;
import util.ConfigLoader;
import java.io.File;
import java.util.concurrent.TimeUnit;

public class EppServerInitializer extends ChannelInitializer<SocketChannel> {

    private final boolean useSSL;

    public EppServerInitializer(boolean useSSL) {
        this.useSSL = useSSL;
    }

    @Override
    protected void initChannel(SocketChannel ch) throws Exception {
        var pipeline = ch.pipeline();

        String clientAuthConfig = ConfigLoader.get("tls.client.auth").toUpperCase(); // default REQUIRE
        ClientAuth clientAuth;

        switch (clientAuthConfig) {
            case "NONE" -> clientAuth = ClientAuth.NONE;
            case "OPTIONAL" -> clientAuth = ClientAuth.OPTIONAL;
            default -> clientAuth = ClientAuth.REQUIRE;
        }

        // --- TLS ---
        if (useSSL) {
            File certChainFile = new File("certs/server.crt");
            File privateKeyFile = new File("certs/server.key");
            File trustCertCollectionFile = new File("certs/cacert.pem"); // CA yang dipercaya

            System.out.println("[SSL] Loading certificate from " + certChainFile.getAbsolutePath());
            System.out.println("[SSL] Loading private key from " + privateKeyFile.getAbsolutePath());
            System.out.println("[SSL] Loading ca from " + trustCertCollectionFile.getAbsolutePath());

            SslContext sslCtx = SslContextBuilder.forServer(certChainFile, privateKeyFile)
                                .trustManager(trustCertCollectionFile)         // CA yang dipercaya
                                .clientAuth(clientAuth)    // KUNCI: biarkan client tidak kirim cert
                                .build();
            pipeline.addLast("ssl", sslCtx.newHandler(ch.alloc()));
            System.out.println("[SSL] SSL handler added to pipeline (TLS active)");
            System.out.println("[DEBUG] TLS client auth mode: " + clientAuth.name());
        } else {
            System.out.println("[SSL] SSL disabled, connection is plaintext");
        }

        // --- Logging handler ---
        pipeline.addLast(new LoggingHandler(LogLevel.INFO));

        // --- Timeout handler: 60s idle read timeout ---
        int idleTimeoutSeconds = Integer.parseInt(ConfigLoader.get("idle.timeout.seconds"));
        pipeline.addLast(new IdleStateHandler(idleTimeoutSeconds, 0, 0, TimeUnit.SECONDS));
        pipeline.addLast(new ChannelDuplexHandler() {
            @Override
            public void userEventTriggered(ChannelHandlerContext ctx, Object evt) throws Exception {
                if (evt instanceof IdleStateEvent) {
                    System.out.println("[TIMEOUT] Closing idle connection: " + ctx.channel().remoteAddress());
                    ctx.writeAndFlush(XmlUtils.buildErrorResponse("Idle timeout")).addListener(f -> ctx.close());
                    SessionManager.removeSessionByChannel(ctx.channel());
                } else {
                    super.userEventTriggered(ctx, evt);
                }
            }
        });


        // --- Frame decoder & encoder for EPP 4-byte prefixed messages ---
        //pipeline.addLast(new EppFrameDecoder());
        //pipeline.addLast(new EppDelimiterFrameDecoder()); 
        pipeline.addLast(new EppHybridFrameDecoder()); 
        pipeline.addLast(new LoggingInterceptorHandler());
        pipeline.addLast(new EppFrameEncoder());

        // Optional: debug handler
        pipeline.addLast("debugLogger", new ChannelInboundHandlerAdapter() {
            @Override
            public void channelRead(ChannelHandlerContext ctx, Object msg) throws Exception {
                System.out.println("[DEBUG] debugLogger received: " + msg.getClass());
                super.channelRead(ctx, msg);
            }
        });

        // --- Main EPP handler ---
        pipeline.addLast(new EppServerHandler());

        // --- Fallback exception handler ---
        pipeline.addLast("exceptionLogger", new ChannelInboundHandlerAdapter() {
            @Override
            public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) throws Exception {
                System.err.println("[PIPELINE ERROR] " + cause.getMessage());
                cause.printStackTrace();
                ctx.close();
            }
        });

        // --- Debug pipeline structure ---
        System.out.println("[DEBUG] Pipeline handler order:");
        pipeline.forEach(entry -> {
            System.out.println(" - " + entry.getKey() + ": " + entry.getValue().getClass().getSimpleName());
        });
    }
}
