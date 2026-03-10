package net;

import io.netty.buffer.ByteBuf;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.ByteToMessageDecoder;

import java.nio.charset.StandardCharsets;
import java.util.List;

public class EppDelimiterFrameDecoder extends ByteToMessageDecoder {

    private static final byte[] END_TAG = "</epp>".getBytes(StandardCharsets.UTF_8);

    @Override
    protected void decode(ChannelHandlerContext ctx, ByteBuf in, List<Object> out) {
        int readerIndex = in.readerIndex();
        int writerIndex = in.writerIndex();

        for (int i = readerIndex; i < writerIndex - END_TAG.length + 1; i++) {
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
                out.add(xml);
                return;
            }
        }

        // No complete frame found yet
    }
}
