const { createApp } = Vue;
const { ElMessage } = ElementPlus;

const app = createApp({
    data() {
        return {
            activeMenu: '/test',
            // 工作流列表
            workflows: [],
            // 测试表单
            testForm: {
                workflow_id: '',
                positive_prompt: '',
                negative_prompt: ''
            },
            testFormRef: null,
            // 工作流预览
            workflowPreview: '',
            selectedWorkflow: null,
            // 测试结果
            testResult: null,
            submitting: false,
            refreshing: false,
            // 图像预览
            imagePreviewVisible: false,
            previewImage: null
        };
    },
    mounted() {
        // 根据当前路径设置activeMenu
        this.activeMenu = window.location.pathname;
        this.loadWorkflowList();
        // 检查URL参数
        const urlParams = new URLSearchParams(window.location.search);
        const workflowId = urlParams.get('workflow_id');
        if (workflowId) {
            this.testForm.workflow_id = parseInt(workflowId);
            this.handleWorkflowChange();
        }
    },
    methods: {
        // 加载工作流列表
        async loadWorkflowList() {
            try {
                const response = await fetch('/api/workflows?page=1&page_size=100');
                const result = await response.json();

                if (result.code === 0) {
                    this.workflows = result.data.workflows || [];
                } else {
                    ElMessage.error('加载工作流列表失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载工作流列表失败:', error);
                ElMessage.error('加载工作流列表失败: ' + error.message);
            }
        },
        // 工作流选择变化
        async handleWorkflowChange() {
            if (!this.testForm.workflow_id) {
                this.selectedWorkflow = null;
                this.workflowPreview = '';
                return;
            }

            try {
                const response = await fetch(`/api/workflows/${this.testForm.workflow_id}`);
                const result = await response.json();

                if (result.code === 0) {
                    this.selectedWorkflow = result.data;
                    this.workflowPreview = CommonUtils.formatJSON(result.data.workflow_json);
                    
                    // 提取并显示原有的提示词
                    const workflow = JSON.parse(result.data.workflow_json);
                    const workflowConfig = {
                        positive_node_id: result.data.positive_node_id,
                        positive_field_name: result.data.positive_field_name || 'text',
                        negative_node_id: result.data.negative_node_id,
                        negative_field_name: result.data.negative_field_name || 'text',
                    };
                    const prompts = this.extractPromptsFromWorkflow(workflow, workflowConfig);
                    
                    if (prompts.positive && !this.testForm.positive_prompt) {
                        // 可以设置placeholder，但Element Plus的placeholder不支持动态更新
                        // 这里可以选择显示提示信息
                    }
                    if (prompts.negative && !this.testForm.negative_prompt) {
                        // 同上
                    }
                } else {
                    ElMessage.error('加载工作流详情失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载工作流详情失败:', error);
                ElMessage.error('加载工作流详情失败: ' + error.message);
            }
        },
        // 从工作流中提取提示词
        extractPromptsFromWorkflow(workflow, workflowConfig) {
            let positivePrompt = '';
            let negativePrompt = '';

            // 如果配置了节点，使用配置的节点提取
            if (workflowConfig && workflowConfig.positive_node_id) {
                const node = workflow[workflowConfig.positive_node_id];
                if (node && node.inputs) {
                    const fieldName = workflowConfig.positive_field_name || 'text';
                    positivePrompt = node.inputs[fieldName] || '';
                }
            }

            if (workflowConfig && workflowConfig.negative_node_id) {
                const node = workflow[workflowConfig.negative_node_id];
                if (node && node.inputs) {
                    const fieldName = workflowConfig.negative_field_name || 'text';
                    negativePrompt = node.inputs[fieldName] || '';
                }
            }

            // 如果没有配置，回退到自动检测
            if (!positivePrompt && !negativePrompt) {
                for (const [nodeId, node] of Object.entries(workflow)) {
                    if (node.class_type === 'CLIPTextEncode') {
                        if (node.inputs && node.inputs.text) {
                            if (!positivePrompt) {
                                positivePrompt = node.inputs.text;
                            } else if (!negativePrompt) {
                                negativePrompt = node.inputs.text;
                                break;
                            }
                        }
                    }
                }
            }

            return { positive: positivePrompt, negative: negativePrompt };
        },
        // 替换工作流中的提示词
        replacePromptsInWorkflow(workflow, positivePrompt, negativePrompt, workflowConfig) {
            const workflowCopy = JSON.parse(JSON.stringify(workflow)); // 深拷贝

            // 如果工作流配置了节点和字段，使用配置
            if (workflowConfig && workflowConfig.positive_node_id && positivePrompt) {
                const positiveNode = workflowCopy[workflowConfig.positive_node_id];
                if (positiveNode && positiveNode.inputs) {
                    const fieldName = workflowConfig.positive_field_name || 'text';
                    positiveNode.inputs[fieldName] = positivePrompt;
                }
            }

            if (workflowConfig && workflowConfig.negative_node_id && negativePrompt) {
                const negativeNode = workflowCopy[workflowConfig.negative_node_id];
                if (negativeNode && negativeNode.inputs) {
                    const fieldName = workflowConfig.negative_field_name || 'text';
                    negativeNode.inputs[fieldName] = negativePrompt;
                }
            }

            return workflowCopy;
        },
        // 提交测试请求
        async handleSubmit() {
            if (!this.$refs.testFormRef) return;

            await this.$refs.testFormRef.validate(async (valid) => {
                if (!valid) return;

                if (!this.selectedWorkflow) {
                    ElMessage.error('请先选择工作流');
                    return;
                }

                this.submitting = true;
                try {
                    // 解析工作流JSON
                    let workflow = JSON.parse(this.selectedWorkflow.workflow_json);
                    
                    // 如果有新的提示词，替换工作流中的提示词
                    const workflowConfig = {
                        positive_node_id: this.selectedWorkflow.positive_node_id,
                        positive_field_name: this.selectedWorkflow.positive_field_name || 'text',
                        negative_node_id: this.selectedWorkflow.negative_node_id,
                        negative_field_name: this.selectedWorkflow.negative_field_name || 'text',
                    };

                    if (this.testForm.positive_prompt || this.testForm.negative_prompt) {
                        workflow = this.replacePromptsInWorkflow(
                            workflow,
                            this.testForm.positive_prompt,
                            this.testForm.negative_prompt,
                            workflowConfig
                        );
                    }

                    // 提交请求
                    const response = await fetch('/api/test/request', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json'
                        },
                        body: JSON.stringify({
                            workflow: workflow
                        })
                    });

                    const result = await response.json();

                    if (result.code === 0) {
                        this.testResult = {
                            request_id: result.data.request_id,
                            status: result.data.status || 'pending',
                            progress: 0
                        };
                        ElMessage.success('测试请求已提交');
                    } else {
                        ElMessage.error('提交失败: ' + result.message);
                    }
                } catch (error) {
                    console.error('提交测试请求失败:', error);
                    ElMessage.error('提交测试请求失败: ' + error.message);
                } finally {
                    this.submitting = false;
                }
            });
        },
        // 刷新状态
        async handleRefreshStatus() {
            if (!this.testResult || !this.testResult.request_id) {
                ElMessage.warning('没有可刷新的任务');
                return;
            }

            this.refreshing = true;
            try {
                const response = await fetch(`/api/tasks/${this.testResult.request_id}`);
                const result = await response.json();

                if (result.code === 0) {
                    const task = result.data;
                    this.testResult = {
                        request_id: task.request_id,
                        status: task.status,
                        progress: task.progress || 0,
                        error: task.error
                    };

                    // 如果有文件信息，解析图像
                    if (task.files_info) {
                        try {
                            const files = JSON.parse(task.files_info);
                            this.testResult.images = files.map(file => ({
                                url: file.url || `/api/image-file/${task.user_id}/${task.request_id}/${file.filename}`,
                                filename: file.filename
                            }));
                        } catch (e) {
                            console.error('解析文件信息失败:', e);
                        }
                    }

                    if (task.status === 'completed') {
                        ElMessage.success('任务已完成');
                    } else if (task.status === 'failed') {
                        ElMessage.error('任务失败');
                    }
                } else {
                    ElMessage.error('获取任务状态失败: ' + result.message);
                }
            } catch (error) {
                console.error('刷新状态失败:', error);
                ElMessage.error('刷新状态失败: ' + error.message);
            } finally {
                this.refreshing = false;
            }
        },
        // 查看任务
        handleViewTask() {
            if (this.testResult && this.testResult.request_id) {
                window.location.href = `/?search=${this.testResult.request_id}`;
            }
        },
        // 清空表单
        handleClear() {
            this.testForm = {
                workflow_id: '',
                positive_prompt: '',
                negative_prompt: ''
            };
            this.workflowPreview = '';
            this.selectedWorkflow = null;
            this.testResult = null;
        },
        // 从文件加载
        handleLoadFromFile() {
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = '.json';
            input.onchange = (e) => {
                const file = e.target.files[0];
                if (file) {
                    const reader = new FileReader();
                    reader.onload = (event) => {
                        try {
                            const content = event.target.result;
                            this.workflowPreview = CommonUtils.formatJSON(content);
                            ElMessage.success('文件加载成功');
                        } catch (error) {
                            ElMessage.error('文件格式错误: ' + error.message);
                        }
                    };
                    reader.readAsText(file);
                }
            };
            input.click();
        },
        // 格式化JSON
        handleFormatJSON() {
            if (!this.workflowPreview) {
                ElMessage.warning('请先输入JSON内容');
                return;
            }
            try {
                this.workflowPreview = CommonUtils.formatJSON(this.workflowPreview);
                ElMessage.success('格式化成功');
            } catch (error) {
                ElMessage.error('JSON格式错误: ' + error.message);
            }
        },
        // 预览图像
        handlePreviewImage(image) {
            this.previewImage = image;
            this.imagePreviewVisible = true;
        },
        // 下载图像
        handleDownloadImage(image) {
            const link = document.createElement('a');
            link.href = image.url;
            link.download = image.filename;
            link.target = '_blank';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            ElMessage.success('开始下载: ' + image.filename);
        },
        // 菜单选择
        handleMenuSelect(index) {
            if (index !== this.activeMenu) {
                window.location.href = index;
            }
        }
    }
});

app.config.globalProperties.CommonUtils = CommonUtils;
app.use(ElementPlus).mount('#app');

