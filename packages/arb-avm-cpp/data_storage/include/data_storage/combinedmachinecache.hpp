/*
 * Copyright 2021, Offchain Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifndef ARB_AVM_CPP_COMBINEDMACHINECACHE_HPP
#define ARB_AVM_CPP_COMBINEDMACHINECACHE_HPP

#include <avm/machine.hpp>
#include <avm_values/bigint.hpp>
#include <data_storage/basicmachinecache.hpp>
#include <data_storage/lrumachinecache.hpp>
#include <data_storage/timedmachinecache.hpp>
#include <data_storage/util.hpp>

class CombinedMachineCache {
   public:
    typedef std::map<uint256_t, std::unique_ptr<Machine>> map_type;
    enum cache_result_status_enum {
        Success,
        UseExisting,
        UseDatabase,
        NotFound,
        TooMuchExecution
    };
    typedef struct CacheResultStruct {
        std::unique_ptr<Machine> machine;
        enum cache_result_status_enum status;
    } CacheResult;

   private:
    std::shared_mutex mutex;
    BasicMachineCache basic;
    LRUMachineCache lru;
    TimedMachineCache timed;
    uint256_t database_load_gas_cost;
    uint256_t max_execution_gas;

   public:
    explicit CombinedMachineCache(const ArbCoreConfig& coreConfig)
        : basic{coreConfig.basic_machine_cache_size},
          lru{coreConfig.lru_machine_cache_size},
          timed{coreConfig.timed_cache_expiration_seconds},
          database_load_gas_cost{coreConfig.checkpoint_load_gas_cost},
          max_execution_gas{coreConfig.checkpoint_max_execution_gas} {}

    void basic_add(std::unique_ptr<Machine> machine);
    void lru_add(std::unique_ptr<Machine> machine);
    void timed_add(std::unique_ptr<Machine> machine);
    size_t basic_size();
    size_t lru_size();
    size_t timed_size();
    CacheResultStruct atOrBeforeGas(uint256_t gas_used,
                                    std::optional<uint256_t> existing_gas_used,
                                    std::optional<uint256_t> database_gas,
                                    bool use_max_execution);
    void reorg(uint256_t next_gas_used);
    [[nodiscard]] uint256_t currentTimeExpired();

   private:
    std::optional<std::reference_wrapper<const Machine>> atOrBeforeGasImpl(
        uint256_t& gas_used);
};

#endif  // ARB_AVM_CPP_COMBINEDMACHINECACHE_HPP
