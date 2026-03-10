package net;

import io.netty.buffer.ByteBuf;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.ByteToMessageDecoder;

import java.nio.charset.StandardCharsets;
import java.util.List;

public class EppHybridFrameDecoder extends ByteToMessageDecoder {

    private static final byte[] END_TAG = "</epp>".getBytes(StandardCharsets.UTF_8);

    @Override
    protected void decode(ChannelHandlerContext ctx, ByteBuf in, List<Object> out) throws Exception {

        // [DEBUG] Current readable bytes
        System.out.println("[HYBRID DECODER] Bytes available: " + in.readableBytes());

        // Mode A: Try length-prefixed first
        if (in.readableBytes() >= 4) {
            in.markReaderIndex();
            int length = in.readInt();

            if (length >= 5 && in.readableBytes() >= (length - 4)) {
                byte[] bytes = new byte[length - 4];
                in.readBytes(bytes);
                String xml = new String(bytes, StandardCharsets.UTF_8);
                System.out.println("[HYBRID DECODER] Parsed using 4-byte header");
                out.add(xml);
                return;
            } else {
                in.resetReaderIndex(); // Not enough, fallback
            }
        }

        // Mode B: Look for "</epp>" delimiter
        int readerIndex = in.readerIndex();
        int writerIndex = in.writerIndex();

        for (int i = readerIndex; i <= writerIndex - END_TAG.length; i++) {
            boolean found = true;
            for (int j = 0; j < END_TAG.length; j++) {
                if (in.getByte(i + j) != END_TAG[j]) {
                    found = false;
                    break;
                }
            }

            if (found) {
                int frameLength = i - readerIndex + END_TAG.length;
                ByteBuf frame = in.readRetainedSlice(frameLength);
                String xml = frame.toString(StandardCharsets.UTF_8);
                System.out.println("[HYBRID DECODER] Parsed using </epp> delimiter");
                out.add(xml);
                return;
            }
        }

        // No complete frame yet
    }
}
