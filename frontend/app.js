// Paste 前端逻辑

const API = '';
let currentFilter = '';
let searchQuery = '';
let allItems = [];
let contextItemId = null;
let lastItemsHash = ''; // 用于检测数据是否变化，避免不必要的重渲染

// === 初始化 ===
document.addEventListener('DOMContentLoaded', () => {
    loadItems();
    loadSettings();
    bindEvents();

    // 每5秒轻量级轮询（有数据变化才重渲染）
    setInterval(pollItems, 5000);
});

function bindEvents() {
    // 侧边栏导航
    document.querySelectorAll('.nav-item').forEach(el => {
        el.addEventListener('click', e => {
            e.preventDefault();
            document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
            el.classList.add('active');
            currentFilter = el.dataset.filter;
            // CSS 驱动标签筛选：仅更新 data-filter 属性，浏览器自动处理显隐
            document.getElementById('content-list').dataset.filter = currentFilter;
            updateEmptyState();
        });
    });

    // 搜索（防抖）
    let searchTimer;
    document.getElementById('search-input').addEventListener('input', e => {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(() => {
            searchQuery = e.target.value.toLowerCase();
            applySearch();
            updateEmptyState();
        }, 200);
    });

    // 清空按钮
    document.getElementById('btn-clear-all').addEventListener('click', () => {
        showConfirm('确定清空所有剪切板记录？收藏项也会被删除！', () => {
            fetch('/api/items', { method: 'DELETE' }).then(() => loadItems());
        });
    });

    document.getElementById('btn-clear-unfav').addEventListener('click', () => {
        showConfirm('确定清空所有非收藏记录？', () => {
            fetch('/api/items?favorites=false', { method: 'DELETE' }).then(() => loadItems());
        });
    });

    // 设置面板
    document.getElementById('btn-settings').addEventListener('click', () => {
        document.getElementById('settings-modal').style.display = 'flex';
    });
    document.getElementById('btn-close-settings').addEventListener('click', () => {
        document.getElementById('settings-modal').style.display = 'none';
    });
    document.getElementById('setting-retention').addEventListener('change', e => {
        fetch('/api/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ defaultRetention: e.target.value })
        });
    });

    // 最小化到状态栏
    document.getElementById('btn-minimize').addEventListener('click', () => {
        if (typeof hideWindow === 'function') {
            hideWindow();
        }
    });

    // 图片预览关闭
    document.getElementById('btn-close-preview').addEventListener('click', () => {
        document.getElementById('image-preview').style.display = 'none';
    });

    // 右键菜单关闭
    document.addEventListener('click', () => {
        document.getElementById('context-menu').style.display = 'none';
    });

    // 右键菜单项
    document.querySelectorAll('.ctx-item').forEach(el => {
        el.addEventListener('click', e => {
            e.preventDefault();
            const action = el.dataset.action;
            if (!contextItemId) return;
            if (action === 'copy') pasteItem(contextItemId);
            if (action === 'favorite') toggleFavorite(contextItemId);
            if (action === 'delete') deleteItem(contextItemId);
            document.getElementById('context-menu').style.display = 'none';
        });
    });

    // 事件委托：列表项点击/右键/收藏（避免每次渲染逐个绑定）
    const container = document.getElementById('content-list');
    container.addEventListener('click', e => {
        const favEl = e.target.closest('.item-favorite');
        if (favEl) {
            e.stopPropagation();
            toggleFavorite(favEl.dataset.id);
            return;
        }
        const card = e.target.closest('.item-card');
        if (card) pasteItem(card.dataset.id);
    });
    container.addEventListener('contextmenu', e => {
        const card = e.target.closest('.item-card');
        if (card) {
            e.preventDefault();
            showContextMenu(e, card.dataset.id);
        }
    });
    container.addEventListener('dblclick', e => {
        const card = e.target.closest('.item-card');
        if (card && card.dataset.type === 'image') {
            showImagePreview(card.dataset.id);
        }
    });
}

function showConfirm(message, onConfirm) {
    const modal = document.getElementById('confirm-modal');
    const messageEl = document.getElementById('confirm-message');
    const okBtn = document.getElementById('btn-confirm-ok');
    const cancelBtn = document.getElementById('btn-confirm-cancel');
    const closeBtn = document.getElementById('btn-close-confirm');

    messageEl.textContent = message;
    modal.style.display = 'flex';

    const cleanup = () => {
        modal.style.display = 'none';
        okBtn.onclick = null;
        cancelBtn.onclick = null;
        closeBtn.onclick = null;
    };

    okBtn.onclick = () => { cleanup(); onConfirm(); };
    cancelBtn.onclick = cleanup;
    closeBtn.onclick = cleanup;
}

// === API 调用 ===
let pollTimer = null;
async function loadItems() {
    try {
        // 始终加载全部数据，过滤在前端完成（标签切换即时响应）
        const res = await fetch('/api/items');
        const items = await res.json();
        // 检测数据是否变化，避免不必要的重渲染
        const hash = items.map(i => i.id + ':' + i.isFavorite).join(',');
        if (hash === lastItemsHash) return; // 数据未变化，跳过渲染
        lastItemsHash = hash;
        allItems = items;
        renderItems();
    } catch (e) {
        console.error('加载失败:', e);
    }
}

// 轻量级轮询：只检查条目数量和最新ID，有变化才加载完整数据
async function pollItems() {
    try {
        const res = await fetch('/api/items');
        const items = await res.json();
        const hash = items.map(i => i.id + ':' + i.isFavorite).join(',');
        if (hash === lastItemsHash) return;
        lastItemsHash = hash;
        allItems = items;
        renderItems();
    } catch (e) {
        // 静默失败，不打扰用户
    }
}

async function loadSettings() {
    try {
        const res = await fetch('/api/settings');
        const settings = await res.json();
        document.getElementById('setting-retention').value = settings.defaultRetention || '30d';
    } catch (e) {
        console.error('加载设置失败:', e);
    }
}

async function pasteItem(id) {
    try {
        await fetch(`/api/paste/${id}`, { method: 'POST' });
        showToast('已复制到剪切板');
    } catch (e) {
        showToast('复制失败');
    }
}

async function toggleFavorite(id) {
    try {
        const res = await fetch(`/api/items/${id}/favorite`, { method: 'POST' });
        const data = await res.json();
        showToast(data.isFavorite ? '已收藏' : '已取消收藏');
        loadItems();
    } catch (e) {
        showToast('操作失败');
    }
}

async function deleteItem(id) {
    try {
        await fetch(`/api/items/${id}`, { method: 'DELETE' });
        showToast('已删除');
        loadItems();
    } catch (e) {
        showToast('删除失败');
    }
}

// === 渲染 ===
// 策略：DOM 节点只创建一次，切换标签/搜索时用 display 控制显隐，
// 避免图片被重新解码（这是卡顿的根因）
const domCache = {}; // id -> {el, item} 已创建的卡片节点

function renderItems() {
    const container = document.getElementById('content-list');
    const allData = allItems || [];

    // 收集当前数据中的所有 ID（用于检测已删除的条目）
    const currentIds = new Set(allData.map(i => String(i.id)));

    // 1. 移除已不存在的条目节点
    Object.keys(domCache).forEach(id => {
        if (!currentIds.has(id)) {
            if (domCache[id].el.parentNode) {
                domCache[id].el.remove();
            }
            delete domCache[id];
        }
    });

    // 2. 为新条目创建 DOM 节点（增量，不全量重建）
    const newIds = [];
    allData.forEach(item => {
        const idStr = String(item.id);
        if (!domCache[idStr]) {
            const tmp = document.createElement('div');
            tmp.innerHTML = renderItemCard(item).trim();
            const el = tmp.firstChild;
            domCache[idStr] = { el, item };
            newIds.push(idStr);
        } else {
            // 更新收藏状态（收藏图标和 data-favorite 属性可能变化）
            const cached = domCache[idStr];
            if (cached.item.isFavorite !== item.isFavorite) {
                const favEl = cached.el.querySelector('.item-favorite');
                if (favEl) favEl.textContent = item.isFavorite ? '⭐' : '☆';
                cached.el.dataset.favorite = item.isFavorite;
                cached.item.isFavorite = item.isFavorite;
            }
        }
    });

    // 3. 如果有新条目，按 allData 顺序重新排列 DOM
    // 只在有新条目时才重排，避免每次轮询都触发 reflow
    if (newIds.length > 0) {
        let prevEl = null;
        allData.forEach(item => {
            const idStr = String(item.id);
            const el = domCache[idStr].el;
            if (prevEl) {
                if (prevEl.nextElementSibling !== el) {
                    prevEl.after(el);
                }
            } else {
                if (container.firstChild !== el) {
                    container.prepend(el);
                }
            }
            prevEl = el;
        });
        // 为新创建的图片节点延迟设置 src
        requestAnimationFrame(() => {
            newIds.forEach(idStr => {
                const cached = domCache[idStr];
                if (cached && cached.item.type === 'image') {
                    const img = cached.el.querySelector('.item-image-thumb');
                    if (img) img.src = `data:image/png;base64,${cached.item.preview}`;
                }
            });
        });
    }

    // 4. 标签筛选由 CSS 驱动（data-filter 属性），搜索筛选用 class 控制
    //    无需 JS 逐项切换 display，避免大量 reflow
    applySearch();
    updateEmptyState();
}

// 搜索筛选：仅切换 class，CSS 控制显隐
function applySearch() {
    const allData = allItems || [];
    for (let i = 0; i < allData.length; i++) {
        const item = allData[i];
        const idStr = String(item.id);
        const cached = domCache[idStr];
        if (!cached) continue;
        const matches = !searchQuery || (item.preview || '').toLowerCase().includes(searchQuery);
        cached.el.classList.toggle('search-hidden', !matches);
    }
}

// 空状态：纯 JS 计算可见数量，不触发 reflow
function updateEmptyState() {
    const container = document.getElementById('content-list');
    const visibleCount = (allItems || []).filter(item => matchesFilter(item)).length;
    let emptyState = container.querySelector('.empty-state');
    if (visibleCount === 0) {
        if (!emptyState) {
            emptyState = document.createElement('div');
            emptyState.className = 'empty-state';
            emptyState.innerHTML = '<p>暂无剪切板记录</p><p class="hint">复制内容后自动保存到这里</p>';
            container.appendChild(emptyState);
        }
    } else if (emptyState) {
        emptyState.remove();
    }
}

function matchesFilter(item) {
    if (currentFilter === 'text' && item.type !== 'text') return false;
    if (currentFilter === 'image' && item.type !== 'image') return false;
    if (currentFilter === 'favorite' && !item.isFavorite) return false;
    if (searchQuery) {
        const preview = (item.preview || '').toLowerCase();
        if (!preview.includes(searchQuery)) return false;
    }
    return true;
}

function renderItemCard(item) {
    const typeIcon = item.type === 'image' ? '🖼️' : '📝';
    const favIcon = item.isFavorite ? '⭐' : '☆';
    const retentionLabel = { '1h': '1小时', '1d': '1天', '7d': '7天', '30d': '30天', 'forever': '永久' }[item.retention] || item.retention;
    const timeStr = formatTime(item.createdAt);

    let previewHtml;
    if (item.type === 'image') {
        // 不在 HTML 中嵌入 base64，渲染后通过 JS 设置 src（避免大字符串拖慢 innerHTML 解析）
        previewHtml = `<img class="item-image-thumb" data-id="${item.id}" alt="图片" loading="lazy" />`;
    } else {
        previewHtml = `<div class="item-preview">${escapeHtml(item.preview || '')}</div>`;
    }

    return `
        <div class="item-card" data-id="${item.id}" data-type="${item.type}" data-favorite="${item.isFavorite}">
            <div class="item-type-badge">${typeIcon}</div>
            <div class="item-body">
                ${previewHtml}
                <div class="item-meta">
                    <span class="item-time">${timeStr}</span>
                    <span class="item-retention">${retentionLabel}</span>
                </div>
            </div>
            <span class="item-favorite" data-id="${item.id}">${favIcon}</span>
        </div>`;
}

// === 辅助函数 ===
function formatTime(isoStr) {
    if (!isoStr) return '';
    const d = new Date(isoStr);
    const now = new Date();
    const diff = (now - d) / 1000;

    if (diff < 60) return '刚刚';
    if (diff < 3600) return Math.floor(diff / 60) + '分钟前';
    if (diff < 86400) return Math.floor(diff / 3600) + '小时前';
    if (diff < 604800) return Math.floor(diff / 86400) + '天前';

    const month = (d.getMonth() + 1).toString().padStart(2, '0');
    const day = d.getDate().toString().padStart(2, '0');
    const hour = d.getHours().toString().padStart(2, '0');
    const min = d.getMinutes().toString().padStart(2, '0');
    return `${month}-${day} ${hour}:${min}`;
}

function escapeHtml(str) {
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function showContextMenu(e, id) {
    contextItemId = id;
    const menu = document.getElementById('context-menu');
    menu.style.display = 'block';
    menu.style.left = e.clientX + 'px';
    menu.style.top = e.clientY + 'px';
}

async function showImagePreview(id) {
    try {
        const res = await fetch(`/api/items/${id}`);
        const data = await res.json();
        const img = document.getElementById('preview-img');
        img.src = `data:image/png;base64,${data.content}`;
        document.getElementById('image-preview').style.display = 'flex';
    } catch (e) {
        showToast('加载图片失败');
    }
}

function showToast(msg) {
    const toast = document.getElementById('toast');
    toast.textContent = msg;
    toast.style.display = 'block';
    setTimeout(() => { toast.style.display = 'none'; }, 2000);
}
