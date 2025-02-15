class GitDifUI {
    constructor() {
        // UI Elements
        this.urlList = document.getElementById('urlList');
        this.commitList = document.getElementById('commitList');
        this.diffViewContent = document.getElementById('diffViewContent');
        this.totalCommits = document.getElementById('totalCommits');
        this.lastChange = document.getElementById('lastChange');
        this.pageInfo = document.getElementById('pageInfo');
        this.prevPageBtn = document.getElementById('prevPage');
        this.nextPageBtn = document.getElementById('nextPage');
        this.domainSelector = document.getElementById('domainSelector');
        this.closeDiffModal = document.getElementById('closeDiffModal');
        this.diffModal = document.getElementById('diffModal');

        // Add URL Modal
        this.addUrlModal = document.getElementById('addUrlModal');
        this.addUrlBtn = document.getElementById('addUrlBtn');
        this.cancelAddUrlBtn = document.getElementById('cancelAddUrl');
        this.addUrlForm = document.getElementById('addUrlForm');

        // Edit URL Modal
        this.editUrlModal = document.getElementById('editUrlModal');
        this.editUrlForm = document.getElementById('editUrlForm');
        this.cancelEditUrlBtn = document.getElementById('cancelEditUrl');

        // State
        this.currentUrl = '';
        this.currentPage = 1;
        this.itemsPerPage = 10;
        this.totalItems = 0;
        
        // Event listeners
        // URL List events
        this.urlList.addEventListener('click', (e) => {
            const editBtn = e.target.closest('button[data-edit]');
            const deleteBtn = e.target.closest('button[data-delete]');

            if (editBtn) {
                const urlInfo = JSON.parse(editBtn.dataset.edit);
                this.showEditUrlModal(urlInfo);
            } else if (deleteBtn) {
                this.deleteUrl(deleteBtn.dataset.delete);
            }
        });

        // Domain selector
        this.domainSelector.addEventListener('change', () => {
            this.currentUrl = this.domainSelector.value;
            this.currentPage = 1;
            if (this.currentUrl) {
                this.loadCommits();
            } else {
                this.commitList.innerHTML = '<tr><td colspan="3" class="text-center py-4">Select a domain to view commits</td></tr>';
            }
        });

        // Add URL Modal events
        this.addUrlBtn.addEventListener('click', () => this.showAddUrlModal());
        this.cancelAddUrlBtn.addEventListener('click', () => this.hideAddUrlModal());
        this.addUrlForm.addEventListener('submit', e => this.handleAddUrl(e));

        // Edit URL Modal events
        this.cancelEditUrlBtn.addEventListener('click', () => this.hideEditUrlModal());
        this.editUrlForm.addEventListener('submit', e => this.handleEditUrl(e));

        // Diff Modal events
        this.closeDiffModal.addEventListener('click', () => this.hideDiffModal());

        // Pagination events
        this.prevPageBtn.addEventListener('click', () => this.changePage(-1));
        this.nextPageBtn.addEventListener('click', () => this.changePage(1));

        // Initialize
        this.loadUrls();
        
        // Auto-refresh
        setInterval(() => this.refreshData(), 10000);
    }

    showAddUrlModal() {
        this.addUrlModal.classList.remove('hidden');
    }

    hideAddUrlModal() {
        this.addUrlModal.classList.add('hidden');
    }

    showEditUrlModal(urlInfo) {
        document.getElementById('editUrlInput').value = urlInfo.url;
        document.getElementById('editInterval').value = Math.floor(urlInfo.interval / 1e9); // Convert from nanoseconds to seconds and ensure whole number
        document.getElementById('editTimeout').value = urlInfo.timeout;
        document.getElementById('editStatus').value = urlInfo.status;
        this.editUrlModal.classList.remove('hidden');
    }

    hideEditUrlModal() {
        this.editUrlModal.classList.add('hidden');
    }

    showDiffModal(diffHtml) {
        this.diffViewContent.innerHTML = diffHtml;
        this.diffModal.classList.remove('hidden');
    }

    hideDiffModal() {
        this.diffModal.classList.add('hidden');
        this.diffViewContent.innerHTML = '';
    }

    async handleAddUrl(e) {
        e.preventDefault();
        const url = document.getElementById('newUrl').value;
        const interval = parseInt(document.getElementById('newInterval').value) * 1e9; // Convert to nanoseconds for Go time.Duration
        const timeout = parseInt(document.getElementById('newTimeout').value);
        const status = document.getElementById('newStatus').value;

        try {
            const response = await fetch('/api/add-url', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ url, interval, timeout, status }),
            });

            if (response.ok) {
                this.hideAddUrlModal();
                this.loadUrls();
                document.getElementById('newUrl').value = '';
                document.getElementById('newInterval').value = '10';
                document.getElementById('newTimeout').value = '30';
                document.getElementById('newStatus').value = 'active';
            } else {
                alert('Failed to add URL');
            }
        } catch (error) {
            console.error('Error adding URL:', error);
            alert('Error adding URL');
        }
    }

    async handleEditUrl(e) {
        e.preventDefault();
        const url = document.getElementById('editUrlInput').value;
        const interval = parseInt(document.getElementById('editInterval').value) * 1e9; // Convert to nanoseconds for Go time.Duration
        const timeout = parseInt(document.getElementById('editTimeout').value);
        const status = document.getElementById('editStatus').value;

        try {
            const response = await fetch('/api/edit-url', {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ url, interval, timeout, status }),
            });

            if (response.ok) {
                this.hideEditUrlModal();
                this.loadUrls();
            } else {
                alert('Failed to update URL');
            }
        } catch (error) {
            console.error('Error updating URL:', error);
            alert('Error updating URL');
        }
    }

    async deleteUrl(url) {
        if (!confirm('Are you sure you want to delete this URL?')) {
            return;
        }

        try {
            const response = await fetch(`/api/delete-url?url=${encodeURIComponent(url)}`, {
                method: 'DELETE',
            });

            if (response.ok) {
                this.loadUrls();
                if (this.currentUrl === url) {
                    this.currentUrl = '';
                    this.commitList.innerHTML = '<tr><td colspan="3" class="text-center py-4">Select a domain to view commits</td></tr>';
                }
            } else {
                alert('Failed to delete URL');
            }
        } catch (error) {
            console.error('Error deleting URL:', error);
            alert('Error deleting URL');
        }
    }

    changePage(delta) {
        const newPage = this.currentPage + delta;
        const maxPage = Math.ceil(this.totalItems / this.itemsPerPage);
        
        if (newPage >= 1 && newPage <= maxPage) {
            this.currentPage = newPage;
            this.loadCommits();
        }
    }

    updatePagination(total) {
        this.totalItems = total;
        const maxPage = Math.ceil(total / this.itemsPerPage);
        const start = (this.currentPage - 1) * this.itemsPerPage + 1;
        const end = Math.min(this.currentPage * this.itemsPerPage, total);
        
        this.pageInfo.textContent = `${start}-${end} of ${total}`;
        this.prevPageBtn.disabled = this.currentPage === 1;
        this.nextPageBtn.disabled = this.currentPage === maxPage;
    }

    async loadUrls() {
        try {
            const response = await fetch('/api/urls');
            const urls = await response.json();
            
            // Update domain selector
            const currentDomain = this.domainSelector.value;
            this.domainSelector.innerHTML = '<option value="">Select Domain</option>' + 
                urls.map(urlInfo => `<option value="${urlInfo.url}">${urlInfo.url}</option>`).join('');
            if (currentDomain) {
                this.domainSelector.value = currentDomain;
            }
            
            this.urlList.innerHTML = urls.length ? urls.map(urlInfo => `
                <tr class="hover:bg-gray-50">
                    <td class="px-4 py-2 text-sm">
                        <span class="text-blue-600">
                            ${urlInfo.url}
                        </span>
                    </td>
                    <td class="px-4 py-2 text-sm">
                        <span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full 
                            ${urlInfo.status === 'active' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'}">
                            ${urlInfo.status}
                        </span>
                    </td>
                    <td class="px-4 py-2 text-sm">
                        <button data-edit='${JSON.stringify(urlInfo)}' 
                                class="text-blue-600 hover:text-blue-800 mr-2">
                            Edit
                        </button>
                        <button data-delete="${urlInfo.url}"
                                class="text-red-600 hover:text-red-800">
                            Delete
                        </button>
                    </td>
                </tr>
            `).join('') : '<tr><td colspan="3" class="text-center py-4">No URLs added yet</td></tr>';
        } catch (error) {
            console.error('Error loading URLs:', error);
            this.urlList.innerHTML = '<tr><td colspan="3" class="text-center py-4 text-red-600">Error loading URLs</td></tr>';
        }
    }

    refreshData() {
        if (this.currentUrl) {
            this.loadCommits();
        }
    }

    async loadCommits() {
        const url = this.currentUrl;
        if (!url) return;

        try {
            const start = (this.currentPage - 1) * this.itemsPerPage;
            const response = await fetch(`/api/commits?url=${encodeURIComponent(url)}&start=${start}&limit=${this.itemsPerPage}`);
            const { commits, total } = await response.json();
            
            this.updatePagination(total);
            this.totalCommits.textContent = total;
            this.lastChange.textContent = commits[0]?.date || 'Never';
            
            this.commitList.innerHTML = commits.length ? commits.map(commit => `
                <tr class="hover:bg-gray-50">
                    <td class="px-6 py-4 whitespace-nowrap text-sm">
                        ${new Date(commit.date).toLocaleString()}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm font-mono">
                        ${commit.hash.substring(0, 8)}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm">
                        <button 
                            data-diff="${commit.hash}"
                            class="bg-blue-500 hover:bg-blue-600 text-white px-3 py-1 rounded text-sm">
                            View Changes
                        </button>
                    </td>
                </tr>
            `).join('') : '<tr><td colspan="3" class="text-center py-4">No commits yet</td></tr>';

            // Add event listener for view diff buttons
            this.commitList.addEventListener('click', (e) => {
                const diffBtn = e.target.closest('button[data-diff]');
                if (diffBtn) {
                    this.viewDiff(diffBtn.dataset.diff);
                }
            });
        } catch (error) {
            console.error('Error loading commits:', error);
            this.commitList.innerHTML = '<tr><td colspan="3" class="text-center py-4 text-red-600">Error loading commits</td></tr>';
        }
    }

    async viewDiff(commitHash) {
        const url = this.currentUrl;
        if (!url || !commitHash) return;

        try {
            const response = await fetch(`/api/diff?url=${encodeURIComponent(url)}&commit=${commitHash}`);
            const diff = await response.text();
            
            const diffHtml = Diff2Html.html(diff, {
                drawFileList: true,
                matching: 'lines',
                outputFormat: 'side-by-side'
            });
            
            this.showDiffModal(diffHtml);
        } catch (error) {
            console.error('Error loading diff:', error);
            this.diffViewContent.innerHTML = '<div class="text-red-600 p-4">Error loading diff</div>';
        }
    }
}

// Initialize the UI
const app = new GitDifUI();
