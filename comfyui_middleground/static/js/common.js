// 共享工具函数
const CommonUtils = {
    // 格式化文件大小
    formatFileSize(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
    },
    
    // 格式化日期
    formatDate(dateStr) {
        if (!dateStr) return '';
        return new Date(dateStr).toLocaleString('zh-CN');
    },
    
    // 截断文本
    truncateText(text, maxLength) {
        if (!text) return '';
        return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
    },
    
    // 获取状态文本
    getStatusText(status) {
        const statusMap = {
            'pending': '待处理',
            'running': '运行中',
            'completed': '已完成',
            'failed': '失败'
        };
        return statusMap[status] || status;
    },
    
    // 获取状态类型
    getStatusType(status) {
        const typeMap = {
            'pending': 'info',
            'running': 'warning',
            'completed': 'success',
            'failed': 'danger'
        };
        return typeMap[status] || '';
    },
    
    // 复制到剪贴板
    async copyToClipboard(text) {
        try {
            await navigator.clipboard.writeText(text);
            return true;
        } catch (error) {
            console.error('复制失败:', error);
            return false;
        }
    },
    
    // 格式化JSON
    formatJSON(jsonStr) {
        try {
            const obj = JSON.parse(jsonStr);
            return JSON.stringify(obj, null, 2);
        } catch (error) {
            return jsonStr;
        }
    }
};

