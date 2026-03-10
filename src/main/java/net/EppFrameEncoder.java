package net;

import io.netty.buffer.ByteBuf;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.MessageToByteEncoder;

import java.nio.charset.StandardCharsets;

public class EppFrameEncoder extends MessageToByteEncoder<String> {
    @Override
    protected void encode(ChannelHandlerContext ctx, String msg, ByteBuf out) {
        if (msg == null || msg.isEmpty()) {
            return; // hindari null pointer atau write kosong
        }

        byte[] bytes = msg.getBytes(StandardCharsets.UTF_8);
        int length = bytes.length + 4; // total length termasuk header

        out.writeInt(length);
        out.writeBytes(bytes);
    }
}
