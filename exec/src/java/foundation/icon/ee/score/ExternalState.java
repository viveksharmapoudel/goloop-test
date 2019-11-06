/*
 * Copyright 2019 ICON Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package foundation.icon.ee.score;

import foundation.icon.ee.ipc.EEProxy;
import foundation.icon.ee.types.Bytes;
import foundation.icon.ee.types.ObjectGraph;
import org.aion.avm.core.IExternalState;
import org.aion.types.AionAddress;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.math.BigInteger;
import java.util.Arrays;

public class ExternalState implements IExternalState {
    private static final Logger logger = LoggerFactory.getLogger(ExternalState.class);

    private final EEProxy proxy;
    private final long blockNumber;
    private final long blockTimestamp;
    private byte[] codeCache;
    private ObjectGraph graphCache;

    ExternalState(EEProxy proxy, byte[] codeBytes, BigInteger blockNumber, BigInteger blockTimestamp) {
        this.proxy = proxy;
        this.codeCache = codeBytes;
        this.blockNumber = blockNumber.longValue();
        this.blockTimestamp = blockTimestamp.longValue() / 1000; // micro to milli conversion
    }

    @Override
    public void commit() {
        logger.trace("[commit]");
        throw new RuntimeException("not implemented");
    }

    @Override
    public void commitTo(IExternalState externalState) {
        logger.trace("[commitTo] {}", externalState);
        throw new RuntimeException("not implemented");
    }

    @Override
    public IExternalState newChildExternalState() {
        logger.trace("[newChildExternalState]");
        throw new RuntimeException("not implemented");
    }

    @Override
    public void createAccount(AionAddress address) {
        logger.trace("[createAccount] {}", address);
        throw new RuntimeException("not implemented");
    }

    @Override
    public boolean hasAccountState(AionAddress address) {
        logger.trace("[hasAccountState] {}", address);
        throw new RuntimeException("not implemented");
    }

    @Override
    public byte[] getCode(AionAddress address) {
        logger.trace("[getCode] {}", address);
        throw new RuntimeException("not implemented");
    }

    @Override
    public void putCode(AionAddress address, byte[] code) {
        throw new RuntimeException("should not be called");
    }

    @Override
    public byte[] getTransformedCode(AionAddress address) {
        logger.trace("[getTransformedCode] {}", address);
        if (codeCache == null) {
            throw new RuntimeException("transformed code not found");
        }
        return codeCache;
    }

    @Override
    public void setTransformedCode(AionAddress address, byte[] code) {
        logger.trace("[setTransformedCode] {} len={}", address, code.length);
        try {
            proxy.setCode(code);
            codeCache = code;
        } catch (IOException e) {
            logger.debug("[setTransformedCode] {}", e.getMessage());
        }
    }

    @Override
    public byte[] getObjectGraph(AionAddress address) {
        try {
            ObjectGraph objGraph = null;
            boolean requestFull = true;
            if (graphCache != null) {
                objGraph = proxy.getObjGraph(false);
                if (graphCache.compareTo(objGraph)) {
                    objGraph = graphCache;
                    requestFull = false;
                }
            }
            if (requestFull) {
                objGraph = proxy.getObjGraph(true);
                graphCache = objGraph;
            }
            logger.trace("[getObjectGraph] len={}", objGraph.getGraphData().length);
            return objGraph.getRawData();
        } catch (IOException e) {
            logger.debug("[getObjectGraph] {}", e.getMessage());
            return null;
        }
    }

    @Override
    public void putObjectGraph(AionAddress address, byte[] data) {
        logger.trace("[putObjectGraph] len={}", data.length);
        try {
            boolean includeGraph = true;
            ObjectGraph objGraph = ObjectGraph.getInstance(data);
            if (graphCache != null && graphCache.compareTo(objGraph)) {
                includeGraph = false;
            }
            proxy.setObjGraph(includeGraph, objGraph);
            graphCache = objGraph;
        } catch (IOException e) {
            logger.debug("[putObjectGraph] {}", e.getMessage());
        }
    }

    @Override
    public void putStorage(AionAddress address, byte[] key, byte[] value) {
        logger.trace("[putStorage] key={} value={}", Bytes.toHexString(key), Bytes.toHexString(value));
        try {
            proxy.setValue(key, value);
        } catch (IOException e) {
            logger.debug("[putStorage] {}", e.getMessage());
        }
    }

    @Override
    public void removeStorage(AionAddress address, byte[] key) {
        logger.trace("[removeStorage] key={}", Bytes.toHexString(key));
        try {
            proxy.setValue(key, null);
        } catch (IOException e) {
            logger.debug("[removeStorage] {}", e.getMessage());
        }
    }

    @Override
    public byte[] getStorage(AionAddress address, byte[] key) {
        try {
            byte[] value = proxy.getValue(key);
            logger.trace("[getStorage] key={} value={}", Bytes.toHexString(key), Bytes.toHexString(value));
            return value;
        } catch (IOException e) {
            logger.debug("[getStorage] {}", e.getMessage());
            return null;
        }
    }

    @Override
    public void deleteAccount(AionAddress address) {
        logger.trace("[deleteStorage] {}", address);
        throw new RuntimeException("not implemented");
    }

    @Override
    public BigInteger getBalance(AionAddress address) {
        try {
            BigInteger balance = proxy.getBalance(address.toAddress());
            logger.trace("[getBalance] {} balance={}", address, balance);
            return balance;
        } catch (IOException e) {
            logger.debug("[getBalance] {}", e.getMessage());
            return BigInteger.ZERO;
        }
    }

    @Override
    public void adjustBalance(AionAddress address, BigInteger amount) {
        logger.trace("[adjustBalance] {} amount={}", address, amount);
        // just ignore this
    }

    @Override
    public BigInteger getNonce(AionAddress address) {
        logger.trace("[getNonce] {}", address);
        throw new RuntimeException("not implemented");
    }

    @Override
    public void incrementNonce(AionAddress address) {
        logger.trace("[incrementNonce] {}", address);
        // just ignore this
    }

    @Override
    public void refundAccount(AionAddress address, BigInteger refund) {
        logger.trace("[refundAccount] {} refund={}", address, refund);
        throw new RuntimeException("not implemented");
    }

    @Override
    public byte[] getBlockHashByNumber(long blockNumber) {
        logger.trace("[getBlockHashByNumber] blockNumber={}", blockNumber);
        throw new RuntimeException("not implemented");
    }

    @Override
    public boolean accountNonceEquals(AionAddress address, BigInteger nonce) {
        logger.trace("[accountNonceEquals] {} nonce={}", address, nonce);
        return true;
    }

    @Override
    public boolean accountBalanceIsAtLeast(AionAddress address, BigInteger amount) {
        logger.trace("[accountBalanceIsAtLeast] {} amount={}", address, amount);
        return true;
    }

    @Override
    public boolean isValidEnergyLimitForCreate(long limit) {
        logger.trace("[isValidEnergyLimitForCreate] limit={}", limit);
        return true;
    }

    @Override
    public boolean isValidEnergyLimitForNonCreate(long limit) {
        logger.trace("[isValidEnergyLimitForNonCreate] limit={}", limit);
        return true;
    }

    @Override
    public boolean destinationAddressIsSafeForThisVM(AionAddress address) {
        logger.trace("[destinationAddressIsSafeForThisVM] {}", address);
        throw new RuntimeException("not implemented");
    }

    @Override
    public long getBlockNumber() {
        logger.trace("[getBlockNumber] ret={}", blockNumber);
        return blockNumber;
    }

    @Override
    public long getBlockTimestamp() {
        logger.trace("[getBlockTimestamp] ret={}", blockTimestamp);
        return blockTimestamp;
    }

    @Override
    public long getBlockEnergyLimit() {
        logger.trace("[getBlockEnergyLimit] ret={}", 0);
        return 0;
    }

    @Override
    public BigInteger getBlockDifficulty() {
        logger.trace("[getBlockDifficulty] ret={}", 0);
        return BigInteger.ZERO;
    }

    @Override
    public AionAddress getMinerAddress() {
        byte[] arr = new byte[AionAddress.LENGTH];
        Arrays.fill(arr, (byte) 0xaa);
        arr[0] = (byte) 0x0; // EOA
        AionAddress miner = new AionAddress(arr);
        logger.trace("[getMinerAddress] ret={}", miner);
        return miner;
    }

    public void log(byte[][] indexed, byte[][] data) {
        try {
            proxy.log(indexed, data);
            logger.trace("[logEvent] {} {}", indexed, data);
        } catch (IOException e) {
            logger.debug("[logEvent] {}", e.getMessage());
        }
    }
}
