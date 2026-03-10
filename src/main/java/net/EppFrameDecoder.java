package net;

import io.netty.buffer.ByteBuf;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.ByteToMessageDecoder;

import java.nio.charset.StandardCharsets;
import java.util.List;

public class EppFrameDecoder extends ByteToMessageDecoder {
    @Override
    protected void decode(ChannelHandlerContext ctx, ByteBuf in, List<Object> out) throws Exception {

        System.out.println("[DEBUG] EppFrameDecoder.decode() called with " + in.readableBytes() + " bytes");
        // EPP messages are prefixed with a 4-byte length header
        if (in.readableBytes() < 4) {
            return; // Not enough data to read length
        }

        in.markReaderIndex(); // mark so we can reset if needed
        int length = in.readInt();

        if (in.readableBytes() < length - 4) {
            in.resetReaderIndex();
            return;
        }

        byte[] bytes = new byte[length - 4];
        in.readBytes(bytes);

        String xml = new String(bytes, StandardCharsets.UTF_8);
        out.add(xml); // Pass decoded message to next handler
        System.out.println("[DEBUG] EppFrameDecoder output: " + xml);
    }
}
