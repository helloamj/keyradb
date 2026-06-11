const API_BASE = 'http://localhost:6380';

const refreshBtn = document.getElementById('refresh-btn');
const flushBtn = document.getElementById('flush-btn');
const opSelect = document.getElementById('op');
const valGroup = document.getElementById('val-group');
const queryForm = document.getElementById('query-form');
const queryResult = document.getElementById('query-result');

const memFill = document.getElementById('mem-fill');
const memVal = document.getElementById('mem-val');
const walSize = document.getElementById('wal-size');
const sstCount = document.getElementById('sst-count');

const memContainer = document.getElementById('memtables-container');
const sstContainer = document.getElementById('sstables-container');

refreshBtn.addEventListener('click', fetchStats);
flushBtn.addEventListener('click', forceFlush);

opSelect.addEventListener('change', (e) => {
    if (e.target.value === 'PUT') {
        valGroup.classList.remove('hidden');
        document.getElementById('value').required = true;
    } else {
        valGroup.classList.add('hidden');
        document.getElementById('value').required = false;
    }
});

queryForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const op = opSelect.value;
    const key = document.getElementById('key').value.trim();
    const val = document.getElementById('value').value.trim();
    
    queryResult.className = 'query-result';
    queryResult.textContent = 'Executing...';
    
    try {
        let res;
        if (op === 'GET') {
            res = await fetch(`${API_BASE}/keys/${key}`);
        } else if (op === 'PUT') {
            res = await fetch(`${API_BASE}/keys/${key}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value: val })
            });
        } else if (op === 'DELETE') {
            res = await fetch(`${API_BASE}/keys/${key}`, { method: 'DELETE' });
        }
        
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Request failed');
        
        queryResult.classList.add('success');
        queryResult.textContent = JSON.stringify(data, null, 2);
        fetchStats();
    } catch (err) {
        queryResult.classList.add('error');
        queryResult.textContent = err.message;
    }
});

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

async function forceFlush() {
    try {
        const res = await fetch(`${API_BASE}/flush`, { method: 'POST' });
        if (!res.ok) throw new Error('Flush failed');
        setTimeout(fetchStats, 500);
    } catch (err) {
        alert(err.message);
    }
}

function createTableHTML(title, isImmutable, keys) {
    const headerClass = isImmutable ? 'immutable' : '';
    const badge = isImmutable ? '<span class="badge">Immutable</span>' : '<span class="badge" style="background: rgba(16, 185, 129, 0.2); color: #6ee7b7; border-color: rgba(16, 185, 129, 0.3)">Active</span>';
    
    let entriesHTML = '';
    if (keys && keys.length > 0) {
        entriesHTML = keys.map(k => `
            <div class="entry-row ${k.deleted ? 'entry-deleted' : ''}">
                <span class="entry-key">${k.key}</span>
                <span class="entry-val">${k.deleted ? 'N/A' : k.value}</span>
            </div>
        `).join('');
    } else {
        entriesHTML = '<div class="entry-row" style="color:var(--text-secondary); justify-content:center;">Empty</div>';
    }

    return `
        <div class="data-table ${headerClass}">
            <div class="table-header">
                ${title}
                ${badge}
            </div>
            <div class="table-entries">
                ${entriesHTML}
            </div>
        </div>
    `;
}

function createSSTableHTML(sst) {
    let entriesHTML = '';
    if (sst.blocks && sst.blocks.length > 0) {
        entriesHTML = sst.blocks.map((b, i) => `
            <div class="entry-row" style="flex-direction:column; align-items:flex-start;">
                <div style="font-size:0.75rem; color:var(--text-secondary); margin-bottom:2px;">Block ${i} (Offset: ${b.offset})</div>
                <div style="display:flex; justify-content:space-between; width:100%;">
                    <span class="entry-key">${b.min_key}</span>
                    <span class="entry-val">${b.max_key}</span>
                </div>
            </div>
        `).join('');
    } else {
        entriesHTML = '<div class="entry-row" style="color:var(--text-secondary); justify-content:center;">No Blocks</div>';
    }

    return `
        <div class="data-table">
            <div class="table-header">
                ${sst.name}
                <span class="badge" style="background: rgba(59, 130, 246, 0.2); color: #93c5fd; border-color: rgba(59, 130, 246, 0.3)">${formatBytes(sst.size)}</span>
            </div>
            <div class="table-entries">
                ${entriesHTML}
            </div>
        </div>
    `;
}

let lastStatsStr = "";

async function fetchStats() {
    try {
        const res = await fetch(`${API_BASE}/api/stats`);
        if (!res.ok) throw new Error('Failed to fetch stats');
        
        const text = await res.text();
        if (text === lastStatsStr) return; 
        lastStatsStr = text;
        
        const data = JSON.parse(text);
        
        const pct = Math.min(100, (data.active_mem_size / data.active_mem_max) * 100);
        memFill.style.width = `${pct}%`;
        memVal.textContent = `${formatBytes(data.active_mem_size)} / ${formatBytes(data.active_mem_max)}`;
        walSize.textContent = formatBytes(data.wal_size);
        sstCount.textContent = data.sstables ? data.sstables.length : 0;
        
        let memHTML = createTableHTML('Memtable (Memory)', false, data.active_keys);
        if (data.immutables) {
            data.immutables.forEach((imm, i) => {
                memHTML += createTableHTML(`Flushing Memtable ${i}`, true, imm);
            });
        }
        memContainer.innerHTML = memHTML;
        
        let sstHTML = '';
        if (data.sstables && data.sstables.length > 0) {
            data.sstables.forEach(sst => {
                sstHTML += createSSTableHTML(sst);
            });
        } else {
            sstHTML = '<div style="color:var(--text-secondary); padding: 1rem;">No SSTables on disk yet.</div>';
        }
        sstContainer.innerHTML = sstHTML;

    } catch (err) {
        console.error(err);
    }
}

valGroup.classList.add('hidden');
fetchStats();
setInterval(fetchStats, 2000);
