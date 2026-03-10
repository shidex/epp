package net;

import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.*;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import util.ConfigLoader;

public class EppServer {
    public static void main(String[] args) throws Exception {
        int port = Integer.parseInt(ConfigLoader.get("server.port", "700"));
        boolean useSSL = Boolean.parseBoolean(ConfigLoader.get("server.ssl.enabled", "true"));

        EventLoopGroup bossGroup = new NioEventLoopGroup(1);
        EventLoopGroup workerGroup = new NioEventLoopGroup();

        try {
            ServerBootstrap b = new ServerBootstrap();
            b.group(bossGroup, workerGroup)
             .channel(NioServerSocketChannel.class)
             .childHandler(new EppServerInitializer(useSSL))
             .option(ChannelOption.SO_BACKLOG, 128)
             .childOption(ChannelOption.SO_KEEPALIVE, true);

            System.out.println("EPP server running on port " + port + " (ssl=" + useSSL + ")");
            ChannelFuture f = b.bind(port).sync();
            f.channel().closeFuture().sync();
        } finally {
            bossGroup.shutdownGracefully();
            workerGroup.shutdownGracefully();
        }
    }
}
