import json
import litellm
from litellm._service_logger import Span
from litellm.integrations.custom_logger import CustomLogger
from litellm.proxy.proxy_server import UserAPIKeyAuth, DualCache
from typing import Any, List, Optional, Literal, Tuple, Union
from litellm._logging import verbose_logger
from litellm.types.llms.openai import AllMessageValues
from litellm.types.guardrails import Role
from litellm.types.utils import (
    LLMResponseTypes,
)
import hashlib

# 立即应用 Usage 类的 patch
def _apply_usage_patch():
    """在模块加载时立即修改 LiteLLM 的 Usage 类"""
    try:
        from litellm.types.utils import Usage
        
        # 检查是否已经被patch过
        if hasattr(Usage, '_kimi_patched'):
            return
        
        # 保存原始的 __init__ 方法
        original_init = Usage.__init__
        
        def patched_init(self, *args, **kwargs):
            verbose_logger.warning(f"************* Usage.__init__ called with kwargs: {list(kwargs.keys())} *************")
            
            # 处理 Kimi 的 cached_tokens 字段
            if "cached_tokens" in kwargs:
                cached_tokens = kwargs["cached_tokens"]
                verbose_logger.warning(f"************* Found cached_tokens={cached_tokens}, mapping to cache_read_input_tokens *************")
                # 映射到标准字段
                kwargs["cache_read_input_tokens"] = cached_tokens
            
            # 调用原始构造函数
            original_init(self, *args, **kwargs)
            
            # 确保 cached_tokens 属性存在（供客户端访问）
            if "cached_tokens" in kwargs:
                setattr(self, "cached_tokens", kwargs["cached_tokens"])
                verbose_logger.warning(f"************* Set cached_tokens attribute: {kwargs['cached_tokens']} *************")
        
        # 应用 patch
        Usage.__init__ = patched_init
        Usage._kimi_patched = True
        verbose_logger.warning("************* Successfully patched Usage class at module load time *************")
        
    except Exception as e:
        verbose_logger.exception(f"Failed to patch Usage class at module load time: {e}")

# 立即执行patch
_apply_usage_patch()

# 修复 ResponseAPIUsage 兼容性的 patch
def _apply_response_api_usage_patch():
    """Patch ResponseAPIUsage 类，使其支持 Langfuse 期望的字段名"""
    try:
        from litellm.types.llms.openai import ResponseAPIUsage
        
        # 使用 object.__getattribute__ 避免触发 patch 检查
        # 检查是否已经被patch过
        try:
            object.__getattribute__(ResponseAPIUsage, '_langfuse_compat_patched')
            return  # 已经 patch 过，直接返回
        except AttributeError:
            pass  # 没有 patch 过，继续
        
        # 保存原始的 __getattribute__ 方法
        original_getattribute = ResponseAPIUsage.__getattribute__
        
        def patched_getattribute(self, name):
            """拦截属性访问，提供兼容字段映射
            
            优先级：
            1. 如果字段本身存在（官方已修复），直接返回
            2. 如果字段不存在但有映射源（input_tokens/output_tokens），返回映射值
            3. 否则抛出 AttributeError
            """
            # 先尝试正常访问（官方修复后会成功）
            try:
                return original_getattribute(self, name)
            except AttributeError:
                # 字段不存在，尝试映射
                if name == 'prompt_tokens':
                    try:
                        return original_getattribute(self, 'input_tokens')
                    except AttributeError:
                        pass  # input_tokens 也不存在，继续抛出原错误
                elif name == 'completion_tokens':
                    try:
                        return original_getattribute(self, 'output_tokens')
                    except AttributeError:
                        pass  # output_tokens 也不存在，继续抛出原错误
                
                # 重新抛出原始 AttributeError
                raise
        
        # 应用 patch
        ResponseAPIUsage.__getattribute__ = patched_getattribute
        ResponseAPIUsage._langfuse_compat_patched = True
        verbose_logger.warning("************* Successfully patched ResponseAPIUsage.__getattribute__ for Langfuse compatibility *************")
        
    except Exception as e:
        verbose_logger.exception(f"Failed to patch ResponseAPIUsage: {e}")

# 立即执行patch
_apply_response_api_usage_patch()

class MyCustomHandler(CustomLogger):
    def __init__(self):
        self.default_weight = 40
        self._patched = False
    
    # 移除之前的 _patch_usage_class 方法，因为我们在模块级别已经patch了
    
    def _patch_bedrock_messages_cache_control(self):
        if self._patched:
            return
            
        try:
            import litellm.litellm_core_utils.prompt_templates.factory as factory_module
            original_bedrock_messages_pt_async = factory_module.BedrockConverseMessagesProcessor._bedrock_converse_messages_pt_async
            bedrock_cache_point = {"type": "default" }
            
            async def patched_bedrock_messages_pt_async(
                messages,
                model,
                llm_provider,
                user_continue_message=None,
                assistant_continue_message=None,
            ):
                verbose_logger.warning("DEBUG: Simplified async bedrock messages processing")
                result = await original_bedrock_messages_pt_async(
                    messages, model, llm_provider, user_continue_message, assistant_continue_message
                )
                if result:
                    last_message = result[-1]
                    if last_message.get("content"):
                        if isinstance(last_message["content"], list):
                            # 在content数组的最后添加一个新的ContentBlock，只包含cachePoint
                            cache_block = {"cachePoint": bedrock_cache_point}
                            last_message["content"].append(cache_block)
                            verbose_logger.warning("DEBUG: Added cache point as separate ContentBlock")
                        else:
                            # 如果content不是列表，在message级别添加cachePoint
                            last_message["cachePoint"] = bedrock_cache_point
                            verbose_logger.warning("DEBUG: Added cache point at message level")
                return result
            
            # 应用patch
            factory_module.BedrockConverseMessagesProcessor._bedrock_converse_messages_pt_async = patched_bedrock_messages_pt_async
            
            self._patched = True
            verbose_logger.warning("Successfully applied simplified bedrock cache_control patch")
            
        except Exception as e:
            verbose_logger.exception(f"Failed to apply simplified bedrock cache_control patch: {e}")
    
    def _get_model_name(self, deployment: dict) -> str:
        model_name = deployment.get("model_name", None)
        return model_name
    
    def _get_model_id(self, deployment: dict) -> str:
        model_id = deployment.get("model_info", {}).get("id", None)
        return model_id
    
    def _get_weight(self, deployment: dict) -> int:
        weight = deployment.get("litellm_params", {}).get("weight", self.default_weight)
        return weight
    
    def _get_enable_cache(self, data: dict) -> bool:
        enable_cache = data.get("metadata", {}).get("enable_cache", False)
        return enable_cache
    
    def _get_session_id(self, data: dict) -> str:
        session_id = data.get("metadata", {}).get("session_id", None)
        return session_id
    
    def _get_cache_control(self):
        return {"type": "ephemeral"}
    
    def _select_deployment(self, session_id: str, model: str, healthy_deployments: List) -> dict:
        if not session_id or not healthy_deployments:
            return healthy_deployments
        
        # 简单策略: healthy_deployments 按照 id 和 weight 排序， 同一个session_id, 取同一个位置的deployment
        availible_deployments = [deployment for deployment in healthy_deployments if self._get_model_name(deployment) == model]
        sorted_deployments = sorted(availible_deployments, key=self._get_model_id)
        
        total_weight = sum(self._get_weight(deployment) for deployment in sorted_deployments)
        if total_weight == 0: # 如果所有权重都为0, 则只考虑index
            total_weight = len(sorted_deployments)
        
        random_num = int(hashlib.md5(session_id.encode()).hexdigest(), 16)
        hash_value = random_num % total_weight
        verbose_logger.warning(f"input length:{len(healthy_deployments)}, availible length:{len(availible_deployments)}, random_num:{random_num}, total_weight:{total_weight}, hash_value:{hash_value}")
        
        calc_weight = 0
        selected = sorted_deployments[0]
        
        for deployment in sorted_deployments:
            weight = self._get_weight(deployment)
            if weight == 0:
                weight = 1  
            calc_weight += weight
            if hash_value < calc_weight:
                selected = deployment
                break
        
        model_name = self._get_model_name(selected)
        model_id = self._get_model_id(selected)
        
        verbose_logger.warning(f"availible length:{len(availible_deployments)},  session_id: {session_id}, model_id: {model_id}, model_name: {model_name}")
        
        return [selected]

    def _custom_calculate_cost(self, usage_dict: dict, litellm_params: dict) ->  Tuple[bool, float]:
        # 防御性检查：litellm_params 可能为 None 或不是 dict
        if not litellm_params or not isinstance(litellm_params, dict):
            return False, 0
        
        # 防御性获取 metadata（可能为 None）
        metadata = litellm_params.get("metadata") or {}
        deployment = metadata.get('deployment', None)
        
        verbose_logger.warning(f"************* deployment: {deployment} *************")
        if deployment:
            try:
                model_info = litellm.get_model_info(deployment)
            except Exception as e:
                verbose_logger.warning(f"************* get_model_info from deployment {deployment} failed. error: {e}. fallback to model_info in litellm_params *************")
                model_info = metadata.get("model_info", None)
        else:
            model_info = metadata.get("model_info", None)
            if model_info is None:
                return False, 0
        verbose_logger.warning(f"************* Model info: {model_info} *************")
        verbose_logger.warning(f"************* Usage dict: {usage_dict} *************")
        
        if model_info is None:
            return False, 0
        
        if model_info.get('input_cost_per_token', None) is None:
            return False, 0
        
        def safe_value(value, default=0):
            if value is None:
                return default
            return value
        
        input_cost_per_token = safe_value(model_info.get('input_cost_per_token', 0))
        output_cost_per_token = safe_value(model_info.get('output_cost_per_token', 0))
        cache_read_cost_per_token = safe_value(model_info.get('cache_read_input_token_cost', 0))
        cache_creation_cost_per_token = safe_value(model_info.get('cache_creation_input_token_cost', 0))
        
        prompt_tokens = safe_value(usage_dict.get('prompt_tokens', 0))
        completion_tokens = safe_value(usage_dict.get('completion_tokens', 0))
        cache_read_tokens = safe_value(usage_dict.get('cache_read_input_tokens', 0))
        cache_creation_tokens = safe_value(usage_dict.get('cache_creation_input_tokens', 0))
        
        actual_input_tokens = prompt_tokens - cache_read_tokens
        input_cost = actual_input_tokens * input_cost_per_token
        cache_read_cost = cache_read_tokens * cache_read_cost_per_token
        cache_creation_cost = cache_creation_tokens * cache_creation_cost_per_token
        output_cost = completion_tokens * output_cost_per_token
        
        total_cost = input_cost + cache_read_cost + cache_creation_cost + output_cost
        return True, total_cost

    def _preprocess_usage_dict(self, usage_obj, model_name: Optional[str] = None, kwargs: dict = None):
        usage_dict = (
            usage_obj.model_dump() 
            if hasattr(usage_obj, 'model_dump') 
            else usage_obj.__dict__
        )
        
        if "prompt_tokens_details" not in usage_dict or usage_dict["prompt_tokens_details"] is None:
            usage_dict["prompt_tokens_details"] = {}

        # AWS Bedrock Claude token修复 - 通过API Base URL AND litellm_model_name识别
        if kwargs:
            litellm_params = kwargs.get('litellm_params', {})
            api_base = litellm_params.get('api_base', '')
            
            # 获取litellm_model_name的几种可能方式
            litellm_model_name = (
                kwargs.get('litellm_model_name', '') or 
                litellm_params.get('litellm_model_name', '') or
                kwargs.get('model', '') or
                litellm_params.get('model', '')
            )
            
            # 通过API Base URL AND litellm_model_name识别AWS Bedrock (两个条件必须同时满足)
            is_aws_bedrock_api = 'bedrock-runtime' in api_base and 'amazonaws.com' in api_base
            is_claude_model = 'claude' in litellm_model_name.lower()
            
            if is_aws_bedrock_api and is_claude_model:
                # 获取原始值
                original_prompt_tokens = usage_dict.get("prompt_tokens", 0)
                cache_creation_tokens = usage_dict.get("cache_creation_input_tokens", 0)
                cache_read_tokens = usage_dict.get("cache_read_input_tokens", 0)
                completion_tokens = usage_dict.get("completion_tokens", 0)
                
                # AWS Bedrock修正: prompt_tokens = 原始prompt_tokens + cache_creation + cache_read
                corrected_prompt_tokens = original_prompt_tokens + cache_creation_tokens + cache_read_tokens
                corrected_total_tokens = completion_tokens + corrected_prompt_tokens
                
                verbose_logger.warning(
                    f"************* AWS Bedrock Claude Token Fix - Conditions Matched *************\n"
                    f"API condition satisfied: {is_aws_bedrock_api} (api_base: {api_base})\n"
                    f"Model condition satisfied: {is_claude_model} (litellm_model_name: {litellm_model_name})\n"
                    f"Original prompt_tokens: {original_prompt_tokens}\n"
                    f"cache_creation_input_tokens: {cache_creation_tokens}\n" 
                    f"cache_read_input_tokens: {cache_read_tokens}\n"
                    f"Corrected prompt_tokens: {original_prompt_tokens} + {cache_creation_tokens} + {cache_read_tokens} = {corrected_prompt_tokens}\n"
                    f"Corrected total_tokens: {completion_tokens} + {corrected_prompt_tokens} = {corrected_total_tokens}"
                )
                
                # 应用修正
                usage_dict["prompt_tokens"] = corrected_prompt_tokens
                usage_dict["total_tokens"] = corrected_total_tokens
            else:
                # 调试：不满足条件时也记录
                verbose_logger.warning(
                    f"************* AWS Bedrock Claude Conditions Not Met *************\n"
                    f"API condition: {is_aws_bedrock_api}\n"
                    f"Model condition: {is_claude_model}\n"
                    f"api_base: {api_base}\n"
                    f"litellm_model_name: {litellm_model_name}"
                )

        # 仅当明确是 kimi-k2-turbo-preview 时做映射
        if model_name == "kimi-k2-turbo-preview":
            # Kimi 的 cached_tokens 可能在根级别或者直接从原始 usage_obj 获取
            kimi_cached_tokens = usage_dict.get("cached_tokens", 0) or getattr(usage_obj, "cached_tokens", 0)
            
            if kimi_cached_tokens and kimi_cached_tokens > 0:
                # 将 Kimi 的 cached_tokens 映射到 LiteLLM 标准字段
                usage_dict["cache_read_input_tokens"] = kimi_cached_tokens
                verbose_logger.warning(
                    f"************* kimi-k2-turbo-preview - cached_tokens: {kimi_cached_tokens} -> cache_read_input_tokens *************"
                )
            else:
                # chunk 模式下，Kimi 可能不返回 cached_tokens，进行推断
                prompt_tokens = usage_dict.get("prompt_tokens", 0)
                verbose_logger.warning(f"************* Kimi inference: prompt_tokens={prompt_tokens}, threshold=100 *************")
                if prompt_tokens > 100:
                    usage_dict["cache_read_input_tokens"] = prompt_tokens
                    verbose_logger.warning(
                        f"************* kimi-k2-turbo-preview chunk - prompt_tokens: {prompt_tokens} -> cache_read_input_tokens *************"
                    )

        cached_tokens = usage_dict.get("cache_read_input_tokens", 0)
        usage_dict["prompt_tokens_details"]["cached_tokens"] = cached_tokens
        return usage_dict

    def _update_logging_object(self, kwargs, usage_dict):
        logging_obj = kwargs.get("standard_logging_object")
        if not logging_obj:
            return
            
        if "hidden_params" not in logging_obj or logging_obj["hidden_params"] is None:
            logging_obj["hidden_params"] = {}
        logging_obj["hidden_params"]["usage_object"] = usage_dict

    def _calculate_and_set_custom_cost(self, kwargs, usage_dict):
        # 确保 litellm_params 是一个 dict
        litellm_params = kwargs.get('litellm_params')
        if not litellm_params or not isinstance(litellm_params, dict):
            litellm_params = {}
        
        origin_cost = kwargs.get('response_cost')
        
        is_valid, custom_cost = self._custom_calculate_cost(usage_dict, litellm_params)
        final_cost = custom_cost if is_valid else origin_cost
        
        verbose_logger.warning(
            f"Cost calculation - Original: {origin_cost}, Custom: {custom_cost}, Final: {final_cost}"
        )
        
        if final_cost:
            kwargs["response_cost"] = final_cost

    def _process_usage_info(self, usage_obj, kwargs, result):
        if not usage_obj:
            return kwargs, result
            
        try:
            # 获取模型名
            model_name = getattr(result, "model", None) or kwargs.get("model") or kwargs.get("litellm_params", {}).get("model")
            verbose_logger.warning(f"************* _process_usage_info model_name: {model_name} *************")
            
            usage_dict = self._preprocess_usage_dict(usage_obj, model_name, kwargs)  # 传递 model_name 和 kwargs
            self._update_logging_object(kwargs, usage_dict)
            self._calculate_and_set_custom_cost(kwargs, usage_dict)
            
            setattr(result, "raw_usage", usage_obj)
            return kwargs, result
        except Exception as e:
            verbose_logger.exception(f"_process_usage_info exception: {e}")
            return kwargs, result

    async def async_logging_hook(
        self, kwargs: dict, result: Any, call_type: str
    ) -> Tuple[dict, Any]:
        verbose_logger.warning(f"************* async_logging_hook *************")
        usage_obj = getattr(result, "usage", None)
        verbose_logger.warning(f"************* usage_obj: {usage_obj} *************")
        
        try:
            kwargs, result = self._process_usage_info(usage_obj, kwargs, result)
            return kwargs, result
        except Exception as e:
            verbose_logger.exception(f"async_logging_hook exception: {e}")
            return kwargs, result
        
    async def async_filter_deployments(
        self,
        model: str,
        healthy_deployments: List,
        messages: Optional[List[AllMessageValues]],
        request_kwargs: Optional[dict] = None,
        parent_otel_span: Optional[Span] = None,
    ) -> List[dict]:
        verbose_logger.info("============== async_filter_deployments start ==============")
        session_id = self._get_session_id(request_kwargs)
        return self._select_deployment(session_id, model, healthy_deployments)

    async def async_pre_call_hook(self, user_api_key_dict: UserAPIKeyAuth, cache: DualCache, data: dict, call_type: Literal[
            "completion",
            "text_completion",
            "embeddings",
            "image_generation",
            "moderation",
            "audio_transcription",
        ]): 
        verbose_logger.info("****************** async_pre_call_hook start ******************************")
        
        enable_cache = self._get_enable_cache(data)
        verbose_logger.warning(f"session_id: {self._get_session_id(data)}, enable_cache: {enable_cache}")
        if not enable_cache:
            return data
        
        # 应用patch
        self._patch_bedrock_messages_cache_control()
        
        # 设置cache_control
        for k, v in data.items():
            if k == "messages":
                for message in v:
                    if message["role"] == Role.SYSTEM.value:
                        message["cache_control"] = self._get_cache_control()
                if len(v) > 0:
                    v[-1]["cache_control"] = self._get_cache_control()
            if k == "tools":
                if len(v) > 0:
                    v[-1]["cache_control"] = self._get_cache_control()
        return data
    
    async def async_pre_call_check(
        self, deployment: dict, parent_otel_span: Optional[Span]
    ) -> Optional[dict]:
        model_name = self._get_model_name(deployment)
        model_id = self._get_model_id(deployment)
        verbose_logger.info(f"************* async_pre_call_check model_name: {model_name}, model_id: {model_id}")
        return deployment


# 创建实例
proxy_handler_instance = MyCustomHandler()